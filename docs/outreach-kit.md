# QCore Outreach Kit

> Purpose: get the first five independent QCore runs without traditional B2B
> selling. The ask is a product test, not a sales meeting.

_Last updated: 2026-06-19._

## One-Line Positioning

QCore is an open-source 4G/5G development core that shows the live signaling
journey and explains why an attach failed, including what to change on each side.

## Who To Contact First

Start with five people, in this order:

1. Two friendlies who can run Docker but are not familiar with QCore.
2. One RAN/device engineer who has fought an attach problem recently.
3. One researcher or lab engineer with UERANSIM, srsRAN, OAI, or real RAN access.
4. One cold-ish second-degree contact who has no reason to be polite about the UX.

Do not ask for a meeting first. Send the brief and ask for a cold run.

## Friendly Email

**Subject:** Can you try QCore and tell me where it breaks?

Hi [Name],

I have been building QCore, an open-source 4G/5G development and test core for
RAN/device developers. The goal is not another feature-count core. It is a core
you can start quickly that shows the signaling journey and explains why an attach
failed and what to change.

Would you spend about 15 minutes trying it cold?

Repository: https://github.com/umairsuperhero/qcore

```bash
git clone https://github.com/umairsuperhero/qcore
cd qcore
make up-5g
```

Then open http://localhost:3000, run the happy path, and trigger one failure.
The short guide is here:
https://github.com/umairsuperhero/qcore/blob/main/docs/try-qcore.md

What I need back:

- Did it run?
- How long until the first useful connection or screen?
- Did the Diagnosis screen explain the failure?
- Where were you confused or stuck?
- Would you use QCore next time you needed a test core?

No polished report is needed. Raw notes, screenshots, or a GitHub issue are ideal.
If you have real gNB/eNB/UE access, there is a separate evidence-capture path, but
the simulator run is already valuable.

Thank you,
Umair

## Follow-Up Email

Send this once, three to five days later. Do not chase after that.

**Subject:** Re: quick QCore cold run

Hi [Name],

One quick follow-up on QCore. Even "I stopped at this exact step" is useful data;
you do not need to complete the run or write a report.

The shortest path is:
https://github.com/umairsuperhero/qcore/blob/main/docs/try-qcore.md

If now is not a good time, no problem at all.

Thanks,
Umair

## X Launch Post

I am opening QCore for its first external testing round.

QCore is an open-source 4G/5G development core for RAN/device engineers. The bet
is developer experience: one-command start, a live signaling trace, and a
plain-English diagnosis when attach or registration fails.

I am looking for five people willing to try it cold and tell me where it breaks.

https://github.com/umairsuperhero/qcore

## X Failure-Led Post

Cellular debugging too often ends with "attach failed" and hundreds of log lines.

QCore is trying a different loop:

failure -> structured trace -> cause + fix -> evidence bundle -> catalog rule ->
better diagnosis for the next developer.

It is open source, and I am looking for honest external runs:
https://github.com/umairsuperhero/qcore

## X Technical Proof Post

QCore's bundled UERANSIM/Linux path now proves:

- native SCTP registration
- protected NAS security flow
- PDU session establishment
- PFCP/UPF tunnel update
- UE ping through `uesimtun0`
- AUTS/SQN resync
- concealed SUCI Profile A

The broader compatibility claim is intentionally narrower: one validated target,
not a conformance matrix. Real-RAN testers welcome.

https://github.com/umairsuperhero/qcore

## Response Routing

- Simulator run: send [`try-qcore.md`](try-qcore.md).
- Real RAN: send [`try-qcore-real-ran.md`](try-qcore-real-ran.md).
- Attach failed: use the `Attach failure` GitHub issue form.
- Diagnosis was wrong: use the `Diagnosis wrong or unhelpful` form.
- New target request: use the `Compatibility target request` form.
- Evidence bundle: run `make capture-real-ran` and share the generated folder after
  scrubbing private subscriber data.

## Claim Safety

Say:

- validated against the bundled UERANSIM Docker/Linux profile
- Profile B is vector-tested
- broader real-RAN compatibility needs per-target evidence
- first external testing round

Do not say:

- 3GPP certified or fully conformant
- compatible with every gNB, UE, or baseband
- production carrier core
- externally validated until an external run actually exists

## Round-One Success

The first round is complete when:

- five people receive the brief
- at least three attempt a run
- at least two reach first connection or produce a useful failure bundle
- every attempt is recorded in `docs/adoption-tracker.csv`
- the highest-friction point becomes a fix, catalog rule, or explicit caveat
