# Try QCore with *your own* RAN — gNB / UE / eNB (the "prove it" round)

> Companion to [try-qcore.md](try-qcore.md). That one uses our bundled simulator.
> **This one is for you if you have real RAN** — a 5G gNB, a UE/baseband, or a 4G
> eNB (or srsRAN). You are the test we can't run ourselves. Your raw notes — even
> "it failed at step 3" — are worth more than a polished report.

## Read this first — what's actually true today

We've validated QCore end-to-end against **one** RAN: the bundled UERANSIM profile,
on Linux, over native SCTP. That's a real result, but it is **not** a claim that
your gNB will attach cleanly. **Pointing your RAN at QCore is new ground**, and the
gaps you hit are exactly the data we're missing. Expect rough edges. That's the
point — we want to find them with you, not pretend they aren't there.

So: **success here is not "it worked."** Success is a captured trace and a clear
note on where reality diverged from what QCore expected.

## You need

- A **Linux host** (the data plane needs `/dev/net/tun` + kernel SCTP; macOS runs
  control-plane only, in SCTP TCP-fallback mode).
- **Docker** and **git**.
- Your **gNB/UE** (5G SA) or **eNB** (4G), reachable on the network to the host.

## The path

```bash
git clone https://github.com/umairsuperhero/qcore && cd qcore
make up-5g     # 5G core (AMF NGAP) + dashboard.  Use `make up` for the 4G EPC.
```

1. Open **http://localhost:3000**. The hero shows the **AMF NGAP endpoint, PLMN, and
   TAC** QCore is advertising. **Start a stopwatch.**
2. **Before you attach**, sanity-check both sides — paste your gNB/UE config into the
   dashboard's *config reconciliation* (or `POST /api/ran-config/reconcile`). It
   compares PLMN, TAC, S-NSSAI, serving-network-name, IMSI, Ki/OPc, DNN, SUCI scheme
   against QCore **before** attach. Fix any mismatch it flags.
3. **Point your gNB/UE (or eNB) at the advertised endpoint** and attach. Stop the
   clock when the dashboard shows **connected** → that's **TTFC**.
4. **If it fails** (likely, first time): watch the **live trace** and the **Diagnosis**
   screen. Start a second clock at the failure; stop it when you understand the
   cause + fix from the screen → **TTRC**. Did QCore name the *actual* cause, or did
   you have to read protocol logs?

> If you've grabbed the evidence harness (`scripts/ci/real-ran-capture.sh`), run it
> instead of stopwatching by hand — it captures the trace, diagnosis, reconcile
> result, and timings into one bundle. Send us that folder.

## Send back — 4 things

- **Target**: gNB/UE/eNB make + model + software version, and 4G or 5G.
- **Your two numbers**: TTFC, and TTRC (or "never got a usable explanation").
- **Where reality diverged**: the first step that didn't match what QCore expected —
  the IE it rejected, the cause it gave (right or wrong), the protocol log line if
  you had to dig. *This is the gold.*
- **One sentence**: would QCore have saved you time vs. your current core? Why / why not.

## A few honest expectations

- **5G gNB**: most likely to get furthest — this is where we've done the most work.
- **4G eNB**: the EPC is solid in our own tests but has **zero** external-eNB
  validation and no bundled 4G simulator — treat this as pure exploration.
- **Anything QCore can't yet decode** is a finding, not a failure on your part. Send
  the trace; that's how the diagnostic catalog gets smarter.

Thank you — you're the first outside hands on this. We'll turn every gap you find
into a fix or a catalog rule, and tell you which.
