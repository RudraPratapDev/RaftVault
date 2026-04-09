"""
BitSecure Analysis Server
Serves ML-powered randomness analysis for the KMS Security Audit tab.
Run: python3 crypto/server.py
Port: 7777
"""

import os
import sys
import json
import math
import traceback
import numpy as np
import pandas as pd
import joblib
from flask import Flask, request, jsonify
from flask_cors import CORS

# ── Feature extraction (inline, no data.py dependency) ──────────────────────

def hex_to_bits(hex_str, n):
    raw = bytes.fromhex(hex_str)
    return np.unpackbits(np.frombuffer(raw, dtype=np.uint8))[:n]

def feat_bit_frequency(bits):
    return float(np.mean(bits))

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
    from numpy.fft import fft
    spectrum = np.abs(fft(bits.astype(float)))[1:len(bits)//2]
    return float(np.mean(spectrum)), float(np.std(spectrum)), float(np.max(spectrum))

def feat_compression_ratio(bits):
    import zlib
    raw        = np.packbits(bits).tobytes()
    compressed = zlib.compress(raw, level=9)
    return float(len(compressed) / len(raw))

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

def extract_features(bits):
    autocorr              = feat_serial_correlation(bits, max_lag=5)
    spec_mean, spec_std, spec_max = feat_spectral(bits)
    long0, long1          = feat_longest_run(bits)
    return {
        "bit_freq"         : feat_bit_frequency(bits),
        "num_runs"         : feat_runs(bits),
        "longest_run_0"    : long0,
        "longest_run_1"    : long1,
        "autocorr_lag1"    : autocorr[0],
        "autocorr_lag2"    : autocorr[1],
        "autocorr_lag3"    : autocorr[2],
        "autocorr_lag4"    : autocorr[3],
        "autocorr_lag5"    : autocorr[4],
        "approx_entropy"   : feat_approximate_entropy(bits, m=1),
        "byte_entropy"     : feat_byte_entropy(bits),
        "spectral_mean"    : spec_mean,
        "spectral_std"     : spec_std,
        "spectral_max"     : spec_max,
        "compression_ratio": feat_compression_ratio(bits),
        "ngram2_chi2"      : feat_ngram_deviation(bits, n=2),
        "ngram3_chi2"      : feat_ngram_deviation(bits, n=3),
        "linear_complexity": feat_linear_complexity(bits),
    }

FEATURE_NAMES = [
    "bit_freq", "num_runs", "longest_run_0", "longest_run_1",
    "autocorr_lag1", "autocorr_lag2", "autocorr_lag3", "autocorr_lag4", "autocorr_lag5",
    "approx_entropy", "byte_entropy", "spectral_mean", "spectral_std", "spectral_max",
    "compression_ratio", "ngram2_chi2", "ngram3_chi2", "linear_complexity"
]

# ── NIST SP 800-22 (3 core tests) ───────────────────────────────────────────

def nist_frequency_test(bits):
    n = len(bits)
    s = np.sum(2 * bits.astype(int) - 1)
    import math
    p = math.erfc(abs(s) / math.sqrt(n * 2))
    return float(p), p >= 0.01

def nist_runs_test(bits):
    import math
    n  = len(bits)
    pi = float(np.mean(bits))
    if abs(pi - 0.5) >= (2 / math.sqrt(n)):
        return 0.0, False
    runs = int(np.sum(bits[:-1] != bits[1:])) + 1
    exp  = 2 * n * pi * (1 - pi)
    denom = math.sqrt(2 * exp * (1 - 2 * pi * (1 - pi)))
    if denom == 0:
        return 0.0, False
    p = math.erfc(abs(runs - exp) / denom)
    return float(p), p >= 0.01

def nist_block_frequency_test(bits, M=128):
    from scipy.stats import chi2 as chi2dist
    n  = len(bits)
    N  = n // M
    if N == 0:
        return 0.0, False
    blocks = [bits[i*M:(i+1)*M] for i in range(N)]
    pi_i   = [float(np.mean(b)) for b in blocks]
    chi_sq = 4 * M * sum((p - 0.5)**2 for p in pi_i)
    p      = float(1 - chi2dist.cdf(chi_sq, df=N))
    return p, p >= 0.01

def run_nist(bits):
    p1, r1 = nist_frequency_test(bits)
    p2, r2 = nist_runs_test(bits)
    p3, r3 = nist_block_frequency_test(bits)
    tests = [
        {"name": "Frequency (Monobit)", "p_value": round(p1, 6), "passed": r1,
         "description": "Tests whether the proportion of 1s is close to 0.5"},
        {"name": "Runs Test",           "p_value": round(p2, 6), "passed": r2,
         "description": "Tests whether oscillations between 0s and 1s are too fast or slow"},
        {"name": "Block Frequency",     "p_value": round(p3, 6), "passed": r3,
         "description": "Tests frequency of 1s within non-overlapping blocks of 128 bits"},
    ]
    passed_count = sum(1 for t in tests if t["passed"])
    return {
        "passed": passed_count == 3,
        "passed_count": passed_count,
        "total": 3,
        "tests": tests
    }

# ── Model loading ────────────────────────────────────────────────────────────

MODELS = {}
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))

def load_models():
    for length in [256, 4096]:
        path = os.path.join(SCRIPT_DIR, f"RandomForest_len{length}.pkl")
        if os.path.exists(path):
            try:
                MODELS[str(length)] = joblib.load(path)
                print(f"[BitSecure] Loaded RF model for len={length}")
            except Exception as e:
                print(f"[BitSecure] Failed to load model len={length}: {e}")
        else:
            print(f"[BitSecure] Model not found: {path}")

load_models()

# ── Flask app ────────────────────────────────────────────────────────────────

app = Flask(__name__)
CORS(app)

@app.route('/health', methods=['GET'])
def health():
    return jsonify({
        "status": "ok",
        "models_loaded": list(MODELS.keys())
    })

@app.route('/analyze', methods=['POST'])
def analyze():
    """
    Analyze a key's raw bytes for randomness quality.
    Expects: { "key_material": "<base64_encoded_key>", "key_id": "..." }
    Returns: full analysis with ML verdict, NIST results, features, bit heatmap
    """
    try:
        data = request.json
        key_material_b64 = data.get("key_material", "")
        key_id = data.get("key_id", "unknown")

        if not key_material_b64:
            return jsonify({"error": "key_material is required"}), 400

        import base64
        raw_bytes = base64.b64decode(key_material_b64)
        bits = np.unpackbits(np.frombuffer(raw_bytes, dtype=np.uint8))

        # Use the best available model length
        actual_len = len(bits)
        if actual_len >= 4096:
            model_len = "4096"
            analysis_bits = bits[:4096]
        else:
            model_len = "256"
            analysis_bits = bits[:256] if actual_len >= 256 else np.pad(bits, (0, 256 - actual_len))

        if model_len not in MODELS:
            return jsonify({"error": f"No model available for {model_len}-bit analysis"}), 500

        # Extract features
        features = extract_features(analysis_bits)

        # ML prediction
        model = MODELS[model_len]
        x = np.array([[features[k] for k in FEATURE_NAMES]])
        probs = model.predict_proba(x)[0]
        prob_weak   = float(probs[1])
        prob_secure = float(probs[0])
        verdict = "WEAK" if prob_weak > 0.5 else "SECURE"

        # Feature importances
        importances = model.feature_importances_
        importance_list = sorted(
            [{"feature": n, "importance": float(v), "feature_value": float(features[n])}
             for n, v in zip(FEATURE_NAMES, importances)],
            key=lambda x: x["importance"], reverse=True
        )

        # NIST tests
        nist = run_nist(analysis_bits)

        # Bit heatmap data — send first 256 bits as rows of 16
        heatmap_bits = analysis_bits[:256].tolist()

        # Byte distribution (first 32 bytes)
        byte_vals = [int(b) for b in raw_bytes[:32]]

        # Autocorrelation profile
        autocorr_profile = [
            {"lag": i+1, "value": round(features[f"autocorr_lag{i+1}"], 6)}
            for i in range(5)
        ]

        return jsonify({
            "key_id": key_id,
            "key_length_bits": actual_len,
            "analysis_length_bits": int(model_len),
            "verdict": verdict,
            "prob_weak": round(prob_weak, 4),
            "prob_secure": round(prob_secure, 4),
            "confidence": round(max(prob_weak, prob_secure), 4),
            "nist": nist,
            "features": {k: round(float(v), 6) for k, v in features.items()},
            "feature_importance": importance_list[:10],
            "heatmap_bits": heatmap_bits,
            "byte_distribution": byte_vals,
            "autocorr_profile": autocorr_profile,
        })

    except Exception as e:
        traceback.print_exc()
        return jsonify({"error": str(e)}), 500

if __name__ == '__main__':
    print("[BitSecure] Starting analysis server on port 7777...")
    app.run(host='0.0.0.0', port=7777, debug=False)
