"""
Step 2: Feature Extraction from PRNG Binary Sequences (Multiprocessing)
"""

import numpy as np
import pandas as pd
from scipy.stats import entropy
from numpy.fft import fft
import zlib
from tqdm import tqdm
from multiprocessing import Pool
import os

# ─────────────────────────────────────────
# UTILITY
# ─────────────────────────────────────────
def hex_to_bits(hex_str, n):
    raw = bytes.fromhex(hex_str)
    return np.unpackbits(np.frombuffer(raw, dtype=np.uint8))[:n]


# ─────────────────────────────────────────
# FEATURE FUNCTIONS
# ─────────────────────────────────────────

def feat_bit_frequency(bits):
    return np.mean(bits)

def feat_runs(bits):
    return int(np.sum(bits[1:] != bits[:-1])) + 1

def feat_longest_run(bits):
    max_0, max_1, cur_0, cur_1 = 0, 0, 0, 0
    for b in bits:
        if b == 0:
            cur_0 += 1; cur_1 = 0
        else:
            cur_1 += 1; cur_0 = 0
        max_0 = max(max_0, cur_0)
        max_1 = max(max_1, cur_1)
    return max_0, max_1

def feat_serial_correlation(bits, max_lag=5):
    n = len(bits)
    mean = np.mean(bits)
    var  = np.var(bits)
    if var == 0:
        return [0.0] * max_lag
    corrs = []
    for lag in range(1, max_lag + 1):
        c = np.mean((bits[:n-lag] - mean) * (bits[lag:] - mean)) / var
        corrs.append(float(c))
    return corrs

def feat_approximate_entropy(bits, m=1):
    n = len(bits)
    def phi(m):
        templates = np.array([bits[i:i+m] for i in range(n - m + 1)])
        counts = []
        for t in templates:
            matches = np.sum(np.all(templates == t, axis=1))
            counts.append(matches / (n - m + 1))
        return np.sum(np.log(counts)) / (n - m + 1)
    try:
        return float(phi(m) - phi(m + 1))
    except:
        return 0.0

def feat_spectral(bits):
    spectrum = np.abs(fft(bits.astype(float)))[1:len(bits)//2]
    return float(np.mean(spectrum)), float(np.std(spectrum)), float(np.max(spectrum))

def feat_compression_ratio(bits):
    raw        = np.packbits(bits).tobytes()
    compressed = zlib.compress(raw, level=9)
    return len(compressed) / len(raw)

def feat_ngram_deviation(bits, n=2):
    total    = len(bits) - n + 1
    expected = total / (2 ** n)
    counts   = {}
    for i in range(total):
        gram = tuple(bits[i:i+n])
        counts[gram] = counts.get(gram, 0) + 1
    chi2 = sum((v - expected)**2 / expected for v in counts.values())
    return float(chi2)

def feat_linear_complexity(bits, max_len=128):
    s = list(bits[:max_len])
    n = len(s)
    C = [1] + [0] * n
    B = [1] + [0] * n
    L, m, b = 0, 1, 1
    for i in range(n):
        d = s[i]
        for j in range(1, L + 1):
            d ^= (C[j] * s[i - j]) % 2
        d %= 2
        if d == 0:
            m += 1
        elif 2 * L <= i:
            T = C[:]
            coef = d * pow(int(b), -1, 2)
            for j in range(m, n + 1):
                C[j] ^= int(coef * B[j - m]) % 2
            L = i + 1 - L
            B = T
            b = d
            m = 1
        else:
            coef = d * pow(int(b), -1, 2)
            for j in range(m, n + 1):
                C[j] ^= int(coef * B[j - m]) % 2
            m += 1
    return float(L)

def feat_byte_entropy(bits):
    raw    = np.packbits(bits).tobytes()
    counts = np.bincount(list(raw), minlength=256).astype(float)
    probs  = counts / counts.sum()
    probs  = probs[probs > 0]
    return float(-np.sum(probs * np.log2(probs)))


# ─────────────────────────────────────────
# MAIN FEATURE EXTRACTOR
# ─────────────────────────────────────────
def extract_features(bits):
    autocorr           = feat_serial_correlation(bits, max_lag=5)
    spec_mean, spec_std, spec_max = feat_spectral(bits)
    long0, long1       = feat_longest_run(bits)

    return {
        "bit_freq"          : feat_bit_frequency(bits),
        "num_runs"          : feat_runs(bits),
        "longest_run_0"     : long0,
        "longest_run_1"     : long1,
        "autocorr_lag1"     : autocorr[0],
        "autocorr_lag2"     : autocorr[1],
        "autocorr_lag3"     : autocorr[2],
        "autocorr_lag4"     : autocorr[3],
        "autocorr_lag5"     : autocorr[4],
        "approx_entropy"    : feat_approximate_entropy(bits, m=1),
        "byte_entropy"      : feat_byte_entropy(bits),
        "spectral_mean"     : spec_mean,
        "spectral_std"      : spec_std,
        "spectral_max"      : spec_max,
        "compression_ratio" : feat_compression_ratio(bits),
        "ngram2_chi2"       : feat_ngram_deviation(bits, n=2),
        "ngram3_chi2"       : feat_ngram_deviation(bits, n=3),
        "linear_complexity" : feat_linear_complexity(bits),
    }


# ─────────────────────────────────────────
# ROW PROCESSOR (must be top-level for multiprocessing)
# ─────────────────────────────────────────
def process_row(row_dict):
    bits = hex_to_bits(row_dict["hex_sequence"], row_dict["seq_length"])
    feats = extract_features(bits)
    feats["generator"] = row_dict["generator"]
    feats["label"]     = row_dict["label"]
    return feats


# ─────────────────────────────────────────
# MAIN
# ─────────────────────────────────────────
if __name__ == "__main__":
    SEQ_LENGTHS = [256, 1024, 4096, 16384]
    INPUT_DIR   = "dataset"
    OUTPUT_DIR  = "features"
    os.makedirs(OUTPUT_DIR, exist_ok=True)

    for seq_len in SEQ_LENGTHS:
        print(f"\nExtracting features for seq_len={seq_len}...")

        df = pd.read_parquet(f"{INPUT_DIR}/prng_dataset_len{seq_len}.parquet")
        records = df.to_dict("records")

        with Pool(processes=os.cpu_count()) as pool:
            feature_rows = list(tqdm(
                pool.imap(process_row, records),
                total=len(records),
                ncols=70
            ))

        feat_df  = pd.DataFrame(feature_rows)
        out_path = f"{OUTPUT_DIR}/features_len{seq_len}.parquet"
        feat_df.to_parquet(out_path, index=False)

        print(f"  Saved: {out_path}  |  Shape: {feat_df.shape}")

    print("\nFeature extraction complete.")