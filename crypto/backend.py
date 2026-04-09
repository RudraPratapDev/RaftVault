import os
import sys
import numpy as np
import pandas as pd
from flask import Flask, request, jsonify
from flask_cors import CORS
import math
import joblib
import shap
import json
import traceback

from data import GENERATORS
from feature_script import extract_features, hex_to_bits

app = Flask(__name__)
CORS(app)

MODELS = {}
EXPLAINERS = {}

def load_models():
    # Preload all RF models and setup TreeExplainers
    for length in [256, 1024, 4096, 16384]:
        path = f"models/RandomForest_len{length}.pkl"
        if os.path.exists(path):
            try:
                model = joblib.load(path)
                MODELS[str(length)] = model
                
                # NOTE: TreeExplainer can be slow to initialize for deep models,
                # but doing it once at startup is okay.
                explainer = shap.TreeExplainer(model)
                EXPLAINERS[str(length)] = explainer
                print(f"Loaded RF model and Explainer for len {length}")
            except Exception as e:
                print(f"Error loading model {length}: {e}")

load_models()

from nistrng import run_all_battery, pack_sequence, SP800_22R1A_BATTERY

def run_nist_tests(bits):
    try:
        packed = pack_sequence(bits)
        results = run_all_battery(packed, SP800_22R1A_BATTERY)
        
        test_results = []
        all_passed = True
        
        for r in results:
            if r is not None:
                passed = r[0].passed
                test_results.append({
                    "test": r[0].name,
                    "passed": passed,
                    "p_value": r[1]
                })
                if not passed:
                    all_passed = False
                    
        return {
            "passed": all_passed,
            "tests": test_results
        }
    except Exception as e:
        print(f"NIST Error: {e}")
        return {"passed": False, "tests": [], "error": str(e)}

@app.route('/api/generators', methods=['GET'])
def get_generators():
    gens = []
    for k, v in GENERATORS.items():
        # v is (func, label)
        gens.append({
            "name": k,
            "type": "weak" if v[1] == 1 else "secure"
        })
    return jsonify({"generators": gens})

@app.route('/api/generate', methods=['POST'])
def generate_sequence():
    data = request.json
    gen_name = data.get('generator', 'LCG')
    seq_len = int(data.get('seq_len', 1024))
    
    if gen_name not in GENERATORS:
        return jsonify({"error": "Invalid generator"}), 400
        
    func, label = GENERATORS[gen_name]
    # Generate sequence
    bits = func(seq_len)
    
    # Pack to hex to send over wire
    packed = np.packbits(bits).tobytes()
    hex_seq = packed.hex()
    
    # Include bits array as string for heatmap if requested
    bits_str = "".join(str(b) for b in bits)
    
    return jsonify({
        "generator": gen_name,
        "seq_len": seq_len,
        "label": int(label),
        "hex_sequence": hex_seq,
        "bits": bits_str
    })

@app.route('/api/analyze', methods=['POST'])
def analyze_sequence():
    data = request.json
    hex_seq = data.get('hex_sequence')
    seq_len = int(data.get('seq_len', 1024))
    run_nist = data.get('run_nist', False)
    
    if str(seq_len) not in MODELS:
        return jsonify({"error": f"No model loaded for length {seq_len}"}), 400
        
    try:
        # Convert hex back to bits
        bits = hex_to_bits(hex_seq, seq_len)
        
        # 1. Extract features
        features = extract_features(bits)
        
        # 2. Format for model prediction
        feature_names = [
            "bit_freq", "num_runs", "longest_run_0", "longest_run_1",
            "autocorr_lag1", "autocorr_lag2", "autocorr_lag3", "autocorr_lag4", "autocorr_lag5",
            "approx_entropy", "byte_entropy", "spectral_mean", "spectral_std", "spectral_max",
            "compression_ratio", "ngram2_chi2", "ngram3_chi2", "linear_complexity"
        ]
        
        # Create 2D array for prediction
        x_pred = pd.DataFrame([{k: features[k] for k in feature_names}])
        
        model = MODELS[str(seq_len)]
        
        # Predict probability of WEAK (class 1)
        probs = model.predict_proba(x_pred)[0]
        prob_weak = float(probs[1]) 
        prob_secure = float(probs[0])
        
        verdict = "WEAK" if prob_weak > 0.5 else "SECURE"
        confidence = prob_weak if prob_weak > 0.5 else prob_secure
        
        # Get Model Feature Importance
        importance = getattr(model, "feature_importances_", None)
        global_importance = []
        if importance is not None:
            # Pair and sort
            impt_pairs = list(zip(feature_names, importance))
            impt_pairs.sort(key=lambda x: x[1], reverse=True)
            global_importance = [{"feature": p[0], "value": float(p[1])} for p in impt_pairs]
        
        response_data = {
            "verdict": verdict,
            "confidence": float(confidence),
            "prob_weak": prob_weak,
            "prob_secure": prob_secure,
            "features": features,
            "global_importance": global_importance[:5]
        }
        
        if run_nist:
            # Run NIST Tests if requested (takes some time)
            nist_result = run_nist_tests(bits)
            response_data["nist_result"] = nist_result
            
        return jsonify(response_data)
        
    except Exception as e:
        traceback.print_exc()
        return jsonify({"error": str(e)}), 500

@app.route('/api/analyze_custom', methods=['POST'])
def analyze_custom():
    data = request.json
    raw_input = data.get('bits', '')
    run_nist = data.get('run_nist', False)
    
    # Strip any non-binary chars
    bits_str = ''.join(c for c in raw_input if c in '01')
    
    if len(bits_str) < 256:
        return jsonify({"error": "Sequence too short. Minimum 256 bits required."}), 400
        
    # Snap to the closest supported model by truncating
    supported = [16384, 4096, 1024, 256]
    target_len = 256
    for slvl in supported:
        if len(bits_str) >= slvl:
            target_len = slvl
            break
            
    # Truncate to target length
    final_bits = bits_str[:target_len]
    seq_len = target_len
    
    # Pack to hex for frontend compatibility
    numeric_bits = np.array([int(b) for b in final_bits], dtype=np.int8)
    packed = np.packbits(numeric_bits).tobytes()
    hex_seq = packed.hex()
    
    try:
        # Extract features
        features = extract_features(numeric_bits)
        
        feature_names = [
            "bit_freq", "num_runs", "longest_run_0", "longest_run_1",
            "autocorr_lag1", "autocorr_lag2", "autocorr_lag3", "autocorr_lag4", "autocorr_lag5",
            "approx_entropy", "byte_entropy", "spectral_mean", "spectral_std", "spectral_max",
            "compression_ratio", "ngram2_chi2", "ngram3_chi2", "linear_complexity"
        ]
        
        x_pred = pd.DataFrame([{k: features[k] for k in feature_names}])
        model = MODELS[str(seq_len)]
        
        probs = model.predict_proba(x_pred)[0]
        prob_weak = float(probs[1]) 
        prob_secure = float(probs[0])
        
        verdict = "WEAK" if prob_weak > 0.5 else "SECURE"
        confidence = prob_weak if prob_weak > 0.5 else prob_secure
        
        importance = getattr(model, "feature_importances_", None)
        global_importance = []
        if importance is not None:
            impt_pairs = list(zip(feature_names, importance))
            impt_pairs.sort(key=lambda x: x[1], reverse=True)
            global_importance = [{"feature": p[0], "value": float(p[1])} for p in impt_pairs]
        
        response_data = {
            "verdict": verdict,
            "confidence": float(confidence),
            "prob_weak": prob_weak,
            "prob_secure": prob_secure,
            "features": features,
            "global_importance": global_importance[:5],
            # Pass back formatted parsed sequence
            "seq_data": {
                "hex_sequence": hex_seq,
                "bits": final_bits,
                "seq_len": seq_len,
                "generator": "Custom User Input",
                "label": -1 # unknown
            }
        }
        
        if run_nist:
            nist_result = run_nist_tests(numeric_bits)
            response_data["nist_result"] = nist_result
            
        return jsonify(response_data)
        
    except Exception as e:
        traceback.print_exc()
        return jsonify({"error": str(e)}), 500

@app.route('/api/shap', methods=['POST'])
def explain_shap():
    data = request.json
    features_dict = data.get('features')
    seq_len = int(data.get('seq_len', 1024))
    
    if str(seq_len) not in EXPLAINERS:
        return jsonify({"error": "Explainer not ready for this length"}), 400
        
    try:
        feature_names = [
            "bit_freq", "num_runs", "longest_run_0", "longest_run_1",
            "autocorr_lag1", "autocorr_lag2", "autocorr_lag3", "autocorr_lag4", "autocorr_lag5",
            "approx_entropy", "byte_entropy", "spectral_mean", "spectral_std", "spectral_max",
            "compression_ratio", "ngram2_chi2", "ngram3_chi2", "linear_complexity"
        ]
        
        x_pred = pd.DataFrame([{k: features_dict[k] for k in feature_names}])
        
        explainer = EXPLAINERS[str(seq_len)]
        raw_shap = explainer.shap_values(x_pred)
        
        if isinstance(raw_shap, list) and len(raw_shap) >= 2:
            class_1_shap = raw_shap[1][0]
        elif isinstance(raw_shap, list):
            class_1_shap = raw_shap[0][0]
        else:
            if len(raw_shap.shape) == 3:
                class_1_shap = raw_shap[0, :, 1]
            else:
                class_1_shap = raw_shap[0]
            
        expected_value = explainer.expected_value
        if isinstance(expected_value, (list, np.ndarray)) and len(expected_value) >= 2:
                expected_value = expected_value[1]
        elif isinstance(expected_value, (list, np.ndarray)) and len(expected_value) == 1:
                expected_value = expected_value[0]
                
        # Prepare waterfall data
        waterfall = []
        for i, name in enumerate(feature_names):
            waterfall.append({
                "feature": name,
                "value": float(features_dict[name]),
                "contribution": float(class_1_shap[i])
            })
            
        # Sort by absolute contribution magnitude for the top 10
        waterfall.sort(key=lambda x: abs(x["contribution"]), reverse=True)
        
        return jsonify({
            "expected_value": float(expected_value),
            "waterfall": waterfall[:10]
        })
        
    except Exception as e:
        traceback.print_exc()
        return jsonify({"error": str(e)}), 500

if __name__ == '__main__':
    # Using 9999 to avoid macOS AirPlay / Control Center conflicts
    app.run(host='0.0.0.0', port=9999, debug=True)
