# AUTS / SQN Resynchronization (5G) — Codex build brief

> Self-contained brief. Read `docs/experience-charter.md` (§3 North Star, §11 scope),
> `CLAUDE.md`/`AGENTS.md`, and `docs/strategy.md` (the "readiness > demand-driven" fork)
> first. Promoted from "Phase 4 / demand-driven" because it's the #1 first-run papercut for
> a real device.

## Mission
The single most frequent first-run papercut for a **physical** UE: restart the core while
the UE stays powered, the UE's stored `SQN-MS` gets ahead of the core's, and the next
authentication is **silently rejected** with a *synch failure*. Today QCore's UDM returns
**HTTP 501** for resync (`pkg/udm/ueau.go`), so a desync'd UE can never re-attach — breaking
the "zero-config, seamless restart" promise. Implement standards-correct **SQN
resynchronization** (TS 33.102 §6.3.5 / TS 33.501) so a synch failure auto-recovers and the
UE attaches on the retry.

## Hard guardrails (read first)
- **No Go toolchain on host** — build/test in Docker:
  ```bash
  docker run --rm -v "$PWD":/src -w /src -v qcore-gomod:/go/pkg/mod golang:1.23 \
    sh -c "go build ./... && go vet ./... && go test -race ./..."
  ```
- **Trust rule:** done only when build + vet + tests pass. The crypto **must be verified
  against 3GPP test vectors**, not just internal round-trip — a wrong constant must fail a test.
- **Scope (charter §11):** 5G resync only (AMF/AUSF/UDM/subscriber + NAS-5G). The crypto
  (f1*/f5*) is shared, so 4G/EPS resync becomes a small follow-up — **note it, don't build it
  here.** No EAP-AKA', no other auth methods.
- **Branch:** `codex/auts-sqn-resync`. One PR. New commits only.
- **⚠️ Do NOT share the working tree with another agent.** If Claude/another agent is active
  in `/Users/umairqayyum/Documents/Software/qcore`, use your own `git worktree`.

## The standards (get the crypto right)
- **AUTS** = Authentication failure parameter, **14 bytes** = `Conc(SQN_MS) || MAC-S`, where
  - `Conc(SQN_MS) = SQN_MS ⊕ f5*(K, RAND)` (6 bytes)
  - `MAC-S = f1*(K, RAND, SQN_MS, AMF)` (8 bytes), with **AMF = 0x0000** for resync (TS 33.102 §6.3.3).
- **f5\*** and **f1\*** are the Milenage **resync** functions (TS 35.206 §4.1) — same K/OPc,
  but a different rotation/constant pair than f5/f1. They are NOT f5/f1; encode them correctly.
- Verify against the **TS 35.207/35.208 resync test vectors** (the test-data sets include
  AUTS / MAC-S examples).

## What already exists — REUSE, don't rebuild
1. **`pkg/subscriber/milenage.go`** — F1–F5 + OPc derivation. **Add `F1Star` + `F5Star`** here
   (verify they don't already exist first).
2. **`pkg/subscriber/aka5g.go`** — `Generate5GAuthVector[WithRAND]`, SQN handling; the
   subscriber store has `IncrementSQN`. Add a way to **advance the stored SQN from a recovered
   `SQN_MS`**.
3. **`pkg/udm/ueau.go`** — `ResynchronizationInfo{AUTS}` is already decoded; today its presence
   returns **501** (`ueau.go:128` area). This is the line to replace with real resync.
4. **`pkg/ausf/ausf.go`** — already carries `ResynchronizationInfo` in the auth request →
   confirm it **forwards** it to UDM `GenerateAuthData`.
5. **`pkg/amf/nas.go`** — `handleAuthenticationFailure` already switches on
   `Cause5GMMSynchFailure` (today it just fails). This is where resync is orchestrated.
6. **`pkg/nas5g/messages.go`** — `AuthenticationFailure` + `EncodeAuthenticationFailure`.
   **Add the AUTS IE** (Authentication failure parameter) to the decode (the UE includes it on
   synch failure); add it to the encode side for the simulator/tests.

## The unit of work (the flow)
1. **NAS-5G:** decode the **AUTS** IE from `AuthenticationFailure` when cause = synch failure
   (UE→AMF). Add it to the encode side so the simulator can produce it.
2. **AMF** (`handleAuthenticationFailure`, `Cause5GMMSynchFailure`): instead of giving up,
   extract AUTS + the original `ue.RAND`, re-call AUSF `CreateUEAuth` with
   `ResynchronizationInfo{AUTS, RAND}`, and on success send a **fresh Authentication Request**
   with the resync'd vector. **Bound it: exactly one resync attempt**, then fail cleanly (no loops).
3. **AUSF:** pass `ResynchronizationInfo` through to UDM `GenerateAuthData`.
4. **UDM** (`ueau.go`) — when `ResynchronizationInfo` is present:
   - `AK* = f5*(K, RAND)`, then `SQN_MS = Conc(SQN_MS) ⊕ AK*`.
   - Verify `MAC-S = f1*(K, RAND, SQN_MS, 0x0000)` against the AUTS MAC-S. **Invalid → reject**
     (do NOT touch SQN; return an auth error, not 501).
   - Valid → **advance the stored SQN** past `SQN_MS` (per the SQN scheme), then generate and
     return a fresh auth vector.
5. **Subscriber store:** persist the advanced SQN.

## Tests (crypto correctness is non-negotiable)
- Go: **f1*/f5* against TS 35.208 resync vectors** (known K/OPc/RAND/SQN_MS → expected
  AK*/MAC-S/AUTS).
- UDM resync: valid AUTS → SQN advanced + new vector; tampered MAC-S → rejected, SQN unchanged.
- AMF: a synch failure carrying AUTS triggers **exactly one** resync re-auth; the success path
  sends a fresh Authentication Request.
- **Preserve all existing tests.**

## Acceptance / Definition of done
- A UE that rejects with synch failure + a valid AUTS makes the core resync and **succeed on
  the retry**; an invalid AUTS is rejected without corrupting SQN.
- `go build ./... && go vet ./... && go test -race ./...` green in `golang:1.23`; f1*/f5* match
  the 3GPP vectors.
- **Real-RAN check (the payoff):** force an SQN desync against UERANSIM — seed the core SQN
  *behind* the UE so UERANSIM (which tracks `SQN-MS`) rejects with synch failure + AUTS — then
  confirm the core resyncs and the UE registers on the retry. Capture via the
  `ueransim-interop` workflow or a documented manual replay. (UERANSIM is known SQN-strict:
  `docs/ueransim-compat.md` seeds the demo SQN at `000000000020` for exactly this reason.)
- Docs reconciled honestly: README, `docs/audit-v1.0.md` revision, `docs/next-phases-plan.md`,
  and a `docs/3gpp-tracking.md` Findings row (real resync interop). Mark ✅ only after the above.

## Verify
```bash
docker run --rm -v "$PWD":/src -w /src -v qcore-gomod:/go/pkg/mod golang:1.23 \
  sh -c "go build ./... && go vet ./... && go test -race ./..."
make verify-fast
```

## Stop condition
Stop when **(A)** synch failure → resync → successful retry works, f1*/f5* are vector-verified,
and the UERANSIM desync replay passes — or **(B)** a concrete blocker is documented with the
exact next step. Do not broaden into EAP-AKA', 4G resync, or other auth methods.

## Next after this
**4G/EPS resync (HSS/MME)** reuses f1*/f5* — a small symmetric follow-up once the 5G path lands.
