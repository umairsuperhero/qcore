# Try QCore — a 5G/4G core you run on your laptop (≈ 15 min)

> For 1–2 external developers giving QCore a first cold run. The point of this round is
> **adoption signal**: can a fresh pair of hands get to "it works" fast, and does QCore
> actually *explain* a failure? Your raw notes are the deliverable — don't polish them.

## Why we're asking

QCore is a development/test core network where **DX is the product**: one command to a
running 5G core, a live dashboard that shows the signaling as it happens, and — when an
attach fails — a plain-English explanation of the **cause *and* the fix on both sides**.
We've validated it ourselves. What we don't have yet is *someone outside the team running
it cold*. Your 15 minutes tell us whether "fast start + explains failures" actually lands.

## You need

- **Docker** and **git**.
- Any OS works for the simulator path below. Point a *real* gNB / UERANSIM at it on **Linux**
  (the bundled data-plane needs `/dev/net/tun` + kernel SCTP).

## The path

```bash
git clone https://github.com/umairsuperhero/qcore && cd qcore
make up-5g          # 5G core + bundled UERANSIM, one command
```

1. **Start a stopwatch when you run `make up-5g`.** Open **http://localhost:3000**. Stop the
   clock the moment the dashboard shows the gNB/core **connected**. → this is your
   **time-to-first-connection (TTFC)**, the number we care about most.
2. Pick **5G SA**, launch the built-in simulator, watch the **live signaling trace** stream,
   and register a UE.
3. **Now break it on purpose** and time the explanation:
   ```bash
   curl -X POST http://localhost:3000/api/simulator/inject/wrong_ki \
     -H "Content-Type: application/json" -d '{"mode":"5g"}'
   ```
   **Start a second stopwatch at injection; stop it when you understand the cause + fix from
   the screen** → **time-to-root-cause (TTRC)**. Did it actually tell you what to change?

(The demo subscriber — 3GPP TS 35.208 Test Set 1 — is seeded automatically on first run.)

## Send back, in 3 bullets

- Your two numbers: **TTFC** and **TTRC**.
- Every moment you were **confused, stuck, or had to read a file** to proceed. *This is the
  gold — be brutal.*
- One sentence: would you reach for this next time you needed a core to test against? Why / why not?

> Stuck on bring-up? Paste the output of `docker compose -f deployments/docker/docker-compose.yml ps`
> and the dashboard URL behavior — that itself is useful TTFC-friction data.
