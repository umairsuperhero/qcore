# QCore — Phase A Work Plan: The Event Model

> Detailed plan for **Phase A** of the build (see `CLAUDE.md`). Phase A is the
> sole gate for Phases B and C. This document is the spec for the work;
> `CLAUDE.md` and `docs/experience-charter.md` remain authoritative on the *why*.

## Goal

Build the **structured event model** — the substrate that the live observability
UI (Phase B) and the AI diagnostic layer (Phase C) both consume.

Today QCore has structured logging and Prometheus metrics, but no event model.
There is no way to follow a single UE's journey across the network functions as
one connected, decoded record. Creating that capability is Phase A.

## Why this is the gate

The "see it work" view and the AI diagnosis both need the same thing: the
complete, correlated story of one registration — every signaling message and
every state change, threaded together and decoded into meaningful fields.
Neither can be built well on ad-hoc log lines. Build this once, build it right,
and both downstream phases stand on it.

## Scope — what Phase A delivers

### 1. The event schema (protocol-agnostic envelope)

A single structured event type, emitted by every network function. Each event
carries:
- a unique event ID and timestamp;
- the source network function;
- the event category — signaling message sent, signaling message received, NF
  state transition, configuration change, or error;
- severity;
- correlation IDs (see §2);
- the protocol (4G S1AP, 4G NAS, 5G NGAP, 5G NAS, SBI, …);
- a structured, decoded payload — protocol-specific detail (message fields,
  cause codes), human-meaningful, not raw bytes.

The **envelope is protocol-agnostic** so 4G and 5G events live under one schema;
the **payload carries protocol-specific detail**. This is what lets the Phase B
UI and the Phase C AI work identically across 4G and 5G.

### 2. Correlation — threading one UE's journey

The hardest and most important design decision in Phase A. A single registration
touches multiple network functions, and the UE's identifier changes along the
way (permanent identity, temporary identities, session/tunnel identifiers). The
event model must mint a single **journey ID** at first contact and carry it
through every subsequent event, mapping each protocol identity onto it.

Test of success: given a journey ID, you can retrieve every event for that UE,
in order, across all network functions.

### 3. The event pipeline — collection, storage, access

QCore's network functions run as separate processes, so events must flow to one
central place. Phase A delivers:
- a **collector** every NF emits to;
- **storage** that keeps events queryable;
- a **query interface** — "give me the full event trace for journey X" (for the
  Phase C AI);
- a **streaming interface** — subscribe to live events as they happen (for the
  Phase B UI).

Claude Code may choose the mechanism (a lightweight message bus, or a central
collector service). The requirement is that all NF events land in one place that
is both queryable and streamable.

### 4. Instrument the 4G network functions

Wire HSS, MME, and SPGW to emit events at every signaling message in/out, every
state transition, every configuration change, and every error. The 4G EPC is
chosen because it runs end-to-end today — it is where the event model gets
proven. (The 5G NFs are instrumented later, on the 5G SA Track, against the same
schema.)

## Relationship to existing logging and metrics

Additive, not a replacement. Structured logging (Logrus) stays for debugging;
Prometheus metrics stay for monitoring. The event model is a new, third signal
layer — the one the observability UI and the AI consume. Do not remove the
others.

## Out of scope for Phase A

- The dashboard or any UI that consumes the stream — Phase B.
- The AI diagnostic layer — Phase C.
- Instrumenting the 5G NFs — 5G SA Track, once those NFs exist.
- 5G-specific configuration validation — 5G SA Track.

## Exit criterion

Run the existing 4G end-to-end test (the UERANSIM attach through MME and SPGW).
The full registration must appear as **one correlated event trace** — every
signaling message and state transition, threaded by a single journey ID,
decoded, and retrievable both as a complete query and as a live stream.

If you can pull "everything that happened for that UE" as one ordered, readable
story, the substrate is real and Phase B can begin.

## Suggested build order within Phase A

1. **Design** the event schema and the correlation strategy. Get this right
   first — it is protocol-agnostic and everything depends on it.
2. **Build** the event pipeline — collector, storage, query interface, streaming
   interface.
3. **Instrument** the 4G NFs (HSS, MME, SPGW).
4. **Verify** against the exit criterion.
