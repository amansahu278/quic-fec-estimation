# FEC Mathematical Formulae in SABRE

This document provides the mathematical formulae for Forward Error Correction (FEC) calculations as implemented in `sabre.py`.

## FEC Parameters & Abbreviations

- **n**: Number of source symbols per FEC block
- **r**: Number of repair symbols per FEC block  
- **s**: Symbol size in bits
- **link_bandwidth**: Link bandwidth (bits/ms)
- **l**: Packet loss rate (0 ≤ l ≤ 1)
- **B**: Payload size in bits

### Key Variables:
- **redundancy_ratio = r/n**: Redundancy ratio
- **coverage_ratio = r/(n+r)**: Coverage ratio  
- **overhead_factor = (n+r)/n = 1 + redundancy_ratio**: Overhead factor

## 1. Effective Payload Bandwidth

### Without FEC:
```
B_eff = link_bandwidth × (1 - l)
```

### With FEC:

**Step 1: Calculate residual loss after FEC protection**
```
residual_loss = max(0, l - coverage_ratio)
```

**Step 2: Calculate bandwidth after loss**
```
bandwidth_after_loss = link_bandwidth × (1 - residual_loss)
```

**Step 3: Apply FEC overhead**
```
B_eff = bandwidth_after_loss / overhead_factor
```

**Combined formula:**
```
B_eff = link_bandwidth × (1 - residual_loss) / overhead_factor
```

Where:
- **coverage_ratio = r/(n+r)**: Fraction of losses that FEC can recover
- **overhead_factor = 1 + redundancy_ratio**: Bandwidth overhead from FEC

## 2. FEC Block Count

```
Blocks = ⌈B / (n × s)⌉
```

## 3. FEC Timing

```
T_enc = Blocks × T_enc_per_block
T_dec = Blocks × T_dec_per_block
```

## 4. Total Download Time

```
T_total = T_enc + T_latency + T_transfer + T_dec
```

Where:
```
T_transfer = B / B_eff
```

## 5. Loss Coverage Analysis

FEC can recover from losses if:
```
coverage_ratio ≥ l
```

Since coverage_ratio = r/(n+r), this gives:
```
r ≥ (l × n) / (1 - l)
```

<!-- ## 6. Optimal FEC Parameters

- **Optimal r**: r = ⌈(l × n) / (1 - l)⌉
- **Trade-off**: Higher r → better coverage, more overhead
- **Balance**: coverage_ratio ≈ l for optimal efficiency -->

## 6. Example Calculation

Given:
- link_bandwidth = 1,000,000 bits/ms, l = 0.1, n = 10, s = 1000 bits
- T_enc_per_block = 1 ms, T_dec_per_block = 2 ms, B = 100,000 bits

Calculations:
```
r = ⌈(0.1 × 10) / (1 - 0.1)⌉ = 2
redundancy_ratio = 2/10 = 0.2
coverage_ratio = 2/12 = 0.167  
overhead_factor = 1 + 0.2 = 1.2
B_eff = 1,000,000 × (1 - max(0, 0.1 - 0.167)) / 1.2 = 833,333 bits/ms
Blocks = ⌈100,000 / (10 × 1000)⌉ = 10
T_enc = 10 × 1 = 10 ms
T_dec = 10 × 2 = 20 ms
T_transfer = 100,000 / 833,333 = 0.12 ms
T_total = 10 + 0.12 + 20 = 30.12 ms
```
