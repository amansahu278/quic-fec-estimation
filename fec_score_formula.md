# FEC Candidate Scoring Formula

At each decision step, we evaluate each candidate \((N,S,R)\) — source symbols, symbol size, and redundancy ratio — and compute a **score** that balances robustness, efficiency, and latency. The candidate with the lowest score is selected.

---

## A) Derived per-candidate quantities

$$
\begin{aligned}
P &= \lceil N R \rceil \quad \text{(repair symbols)} \\
T &= N + P \quad \text{(total symbols)} \\
o &= \frac{P}{T} = \frac{R}{1+R} \quad \text{(overhead fraction)} \\
B_{blk} &= T \cdot S \quad \text{(bytes per FEC block)}
\end{aligned}
$$

---

## B) Runtime signals (observed/estimated)

- \(p\): smoothed loss rate (EWMA)
- \(B\): buffer level (seconds)
- \(G\): goodput / available throughput (bits/s)
- \(R_{play}\): current playback bitrate (bits/s)
- **Headroom:** $$(h = \frac{G - R_{play}}{\max(R_{play}, \epsilon)})$$

**Buffer capping:**
\[
$B_{eff} = \min(B, B_{sat})$
\]

---

## C) Penalty 1 — Loss-robustness


Binomial normal approximation with continuity correction:

$$z = \frac{P + 0.5 - T p}{\sqrt{T p (1-p)}} \quad \text{(safety z-margin)}$$

Target safety margin increases when buffer is small and decreases with headroom:

$$z_{tgt}(B_{eff},h) = z_{min} + \alpha_B \max(0, B_{crit} - B_{eff}) - \alpha_h \min(h, h_{cap})$$

Note: 
$+ \alpha_B \max(0, B_{crit} - B_{eff})$ -> Allows for more safety if buffer is low
$- \alpha_h \min(h, h_{cap})$ -> We should be more lenient if headroom is high


Loss penalty:

$$pen_{loss} = [\max(0, z_{tgt} - z)]^{\beta}$$

This ensures loss governs decisions when either buffer is small or headroom is thin—even with a large buffer.

---

## D) Penalty 2 — Overhead

We will allow some overhead for protection, scaled by both Buffer and headroom. Thus, allowed “free” overhead:

$$o_{free}(B_{eff},h) = \min(o_{cap}, o_0 + k_B B_{eff} + k_h \min(h, h_{cap}))$$

Excess overhead and penalty:

$$o_{ex} = \max(0, o - o_{free}), \quad pen_{over} = o_{ex}^{\alpha}$$

---

## E) Penalty 3 — Blockization / latency

Extremely large blocks can be risky when the buffer is low, because you wait longer to fill/transmit one FEC decision unit
Approximate block transmit time:

$$t_{blk} = \frac{8 T S}{\max(G, \epsilon)}$$

We penalize large $t_{blk}$ relative to buffer
Penalty (starts when $(t_{blk} > \eta B_{eff})$):

$$pen_{blk} = \min(1, \max(0, \frac{t_{blk}}{\eta B_{eff}} - 1))$$

---

## F) Weighted Score

$$Score(N,S,R) = w_{loss} \cdot pen_{loss} + w_{over} \cdot pen_{over} + w_{blk} \cdot pen_{blk}$$


Weights:


$$\begin{aligned}
w_{loss} &= w_{loss,min} + \lambda_p \min(p, p_{cap}) \\
w_{over} &= w_{over,min} + \lambda_B \frac{B_{eff}}{B_{sat}} + \lambda_h \min(h, h_{cap}) \\
w_{blk} &= w_{blk,min} + \lambda_{risk} \max(0, 1 - \frac{B_{eff}}{B_{crit}}) + \lambda_{h^-} \max(0, -\min(h,0))
\end{aligned}
$$

---

## G) Constants / Tunable Parameters

| Parameter | Meaning | Typical Value |
|:--|:--|:--|
| **B_sat** | buffer saturation limit | 6 s |
| **B_crit** | critical buffer (below which safety increases) | 3 s |
| **h_cap** | headroom cap | 2.0 |
| **z_min** | base safety z | 1.0 |
| **α_B** | buffer sensitivity | 0.5 s⁻¹ |
| **α_h** | headroom sensitivity | 0.5 |
| **β** | loss convexity | 1.4 |
| **o₀** | baseline free overhead | 0.03 |
| **k_B** | overhead per buffer sec | 0.01 |
| **k_h** | overhead per headroom unit | 0.03 |
| **o_cap** | max allowed overhead | 0.35 |
| **α** | overhead convexity | 1.5 |
| **η** | block-time fraction of buffer | 0.5 |
| **Weights** | — | — |
| w_loss,min | 0.6 |  |
| λ_p | 4.0 |  |
| p_cap | 0.15 |  |
| w_over,min | 0.3 |  |
| λ_B | 0.5 |  |
| λ_h | 0.4 |  |
| w_blk,min | 0.3 |  |
| λ_risk | 0.6 |  |
| λ_{h^-} | 0.6 |  |

---

## H) Constraints & Tie-breaking

- Reject if \(o > o_{cap}\) or \(t_{blk} > 1.5 B_{eff}\)
- Reject if \(z < 0\) and \(B_{eff} < 1s\)
- Tie-break order: lower o → larger S → larger N (unless buffer < B_crit)
- Apply hysteresis: switch only if new score < old × (1−δ), with δ ∈ [0.05, 0.15]
- Minimum dwell: one segment before re-evaluation

---

## I) Intuitive Summary

| Condition | Dominant Term | Expected Decision |
|:--|:--|:--|
| Low buffer, high loss | loss + block | ↑R, ↓N,S |
| High buffer, limited bandwidth | overhead | ↓R |
| High buffer + high headroom | all small | ↑N,S |
| High loss + good bandwidth | loss | ↑R |
| Low loss + low headroom | overhead | ↓R |

---

This formulation ensures the FEC controller balances **reliability**, **efficiency**, and **latency realism** while adapting continuously to network and buffer conditions.
