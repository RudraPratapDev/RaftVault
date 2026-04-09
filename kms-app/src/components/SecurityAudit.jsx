import { useState, useEffect } from 'react';
import { useAuth } from '../context/AuthContext';
import { apiFetch } from '../utils/api';
import {
  ShieldCheck, ShieldAlert, RefreshCw, ChevronDown,
  Activity, BarChart2, Cpu, AlertTriangle, CheckCircle2, XCircle, Info
} from 'lucide-react';

// ── Bit Heatmap ──────────────────────────────────────────────────────────────
function BitHeatmap({ bits }) {
  if (!bits || bits.length === 0) return null;
  const rows = [];
  for (let i = 0; i < 16; i++) {
    rows.push(bits.slice(i * 16, i * 16 + 16));
  }
  return (
    <div>
      <div className="text-[10px] font-bold text-gray-400 tracking-widest uppercase mb-2">
        Bit Distribution Heatmap (256 bits · 16×16)
      </div>
      <div className="inline-grid gap-0.5" style={{ gridTemplateColumns: 'repeat(16, 1fr)' }}>
        {bits.slice(0, 256).map((bit, i) => (
          <div
            key={i}
            title={`bit ${i}: ${bit}`}
            className={`w-3.5 h-3.5 rounded-sm ${bit === 1 ? 'bg-indigo-500' : 'bg-gray-100'}`}
          />
        ))}
      </div>
      <div className="flex items-center gap-4 mt-2">
        <div className="flex items-center gap-1.5 text-[10px] text-gray-400">
          <div className="w-3 h-3 rounded-sm bg-indigo-500" /> 1-bit
        </div>
        <div className="flex items-center gap-1.5 text-[10px] text-gray-400">
          <div className="w-3 h-3 rounded-sm bg-gray-100 border border-gray-200" /> 0-bit
        </div>
      </div>
    </div>
  );
}

// ── Byte Distribution Bar Chart ───────────────────────────────────────────────
function ByteDistribution({ bytes }) {
  if (!bytes || bytes.length === 0) return null;
  const max = Math.max(...bytes, 1);
  return (
    <div>
      <div className="text-[10px] font-bold text-gray-400 tracking-widest uppercase mb-3">
        Byte Value Distribution (first 32 bytes)
      </div>
      <div className="flex items-end gap-0.5 h-16">
        {bytes.map((v, i) => (
          <div key={i} className="flex-1 flex flex-col items-center gap-0.5 group relative">
            <div
              className="w-full bg-indigo-400 rounded-t-sm transition-all group-hover:bg-indigo-600"
              style={{ height: `${(v / max) * 56}px`, minHeight: '2px' }}
            />
            <div className="absolute -top-6 left-1/2 -translate-x-1/2 bg-gray-800 text-white text-[9px] px-1.5 py-0.5 rounded opacity-0 group-hover:opacity-100 whitespace-nowrap z-10 pointer-events-none">
              {v}
            </div>
          </div>
        ))}
      </div>
      <div className="flex justify-between text-[9px] text-gray-400 mt-1">
        <span>byte 0</span>
        <span>byte 31</span>
      </div>
    </div>
  );
}

// ── Autocorrelation Chart ─────────────────────────────────────────────────────
function AutocorrChart({ profile }) {
  if (!profile || profile.length === 0) return null;
  const maxAbs = Math.max(...profile.map(p => Math.abs(p.value)), 0.01);
  return (
    <div>
      <div className="text-[10px] font-bold text-gray-400 tracking-widest uppercase mb-3">
        Serial Autocorrelation (lags 1–5)
      </div>
      <div className="space-y-2">
        {profile.map(({ lag, value }) => {
          const pct = Math.abs(value) / maxAbs * 100;
          const isHigh = Math.abs(value) > 0.1;
          return (
            <div key={lag} className="flex items-center gap-3">
              <span className="text-[10px] font-mono text-gray-500 w-8">lag{lag}</span>
              <div className="flex-1 bg-gray-100 rounded-full h-2 relative overflow-hidden">
                <div
                  className={`h-full rounded-full transition-all ${isHigh ? 'bg-amber-400' : 'bg-emerald-400'}`}
                  style={{ width: `${pct}%` }}
                />
              </div>
              <span className={`text-[10px] font-mono w-16 text-right ${isHigh ? 'text-amber-600' : 'text-gray-500'}`}>
                {value.toFixed(4)}
              </span>
            </div>
          );
        })}
      </div>
      <p className="text-[10px] text-gray-400 mt-2">
        Values near 0 indicate independence between bits. High values (amber) suggest structure.
      </p>
    </div>
  );
}

// ── Feature Importance Bars ───────────────────────────────────────────────────
function FeatureImportance({ data }) {
  if (!data || data.length === 0) return null;
  const max = Math.max(...data.map(d => d.importance), 0.001);

  const LABELS = {
    bit_freq: 'Bit Frequency',
    num_runs: 'Run Count',
    longest_run_0: 'Longest 0-Run',
    longest_run_1: 'Longest 1-Run',
    autocorr_lag1: 'Autocorr Lag 1',
    autocorr_lag2: 'Autocorr Lag 2',
    autocorr_lag3: 'Autocorr Lag 3',
    autocorr_lag4: 'Autocorr Lag 4',
    autocorr_lag5: 'Autocorr Lag 5',
    approx_entropy: 'Approx Entropy',
    byte_entropy: 'Byte Entropy',
    spectral_mean: 'Spectral Mean',
    spectral_std: 'Spectral Std',
    spectral_max: 'Spectral Peak',
    compression_ratio: 'Compression Ratio',
    ngram2_chi2: '2-gram χ²',
    ngram3_chi2: '3-gram χ²',
    linear_complexity: 'Linear Complexity',
  };

  return (
    <div>
      <div className="text-[10px] font-bold text-gray-400 tracking-widest uppercase mb-3">
        Top Model Features (Random Forest importance)
      </div>
      <div className="space-y-2.5">
        {data.slice(0, 8).map(({ feature, importance, feature_value }) => (
          <div key={feature}>
            <div className="flex justify-between text-[10px] mb-1">
              <span className="text-gray-600 font-medium">{LABELS[feature] || feature}</span>
              <span className="text-gray-400 font-mono">{feature_value?.toFixed(4)}</span>
            </div>
            <div className="bg-gray-100 rounded-full h-1.5 overflow-hidden">
              <div
                className="h-full bg-indigo-500 rounded-full"
                style={{ width: `${(importance / max) * 100}%` }}
              />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

// ── NIST Results ──────────────────────────────────────────────────────────────
function NistResults({ nist }) {
  if (!nist) return null;
  return (
    <div>
      <div className="flex items-center gap-2 mb-3">
        <div className="text-[10px] font-bold text-gray-400 tracking-widest uppercase">
          NIST SP 800-22 Tests
        </div>
        <span className={`text-[9px] font-bold px-2 py-0.5 rounded-full uppercase tracking-wider ${
          nist.passed ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-600'
        }`}>
          {nist.passed_count}/{nist.total} passed
        </span>
      </div>
      <div className="space-y-2">
        {nist.tests.map((t, i) => (
          <div key={i} className="flex items-start gap-3 p-3 rounded-lg bg-gray-50 border border-gray-100">
            <div className="mt-0.5 shrink-0">
              {t.passed
                ? <CheckCircle2 size={14} className="text-emerald-500" />
                : <XCircle size={14} className="text-red-400" />}
            </div>
            <div className="flex-1 min-w-0">
              <div className="text-xs font-semibold text-gray-800">{t.name}</div>
              <div className="text-[10px] text-gray-400 mt-0.5">{t.description}</div>
            </div>
            <div className="text-right shrink-0">
              <div className={`text-[10px] font-bold ${t.passed ? 'text-emerald-600' : 'text-red-500'}`}>
                {t.passed ? 'PASS' : 'FAIL'}
              </div>
              <div className="text-[10px] text-gray-400 font-mono">p={t.p_value}</div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

// ── Verdict Badge ─────────────────────────────────────────────────────────────
function VerdictBadge({ verdict, confidence, probWeak, probSecure }) {
  const isSecure = verdict === 'SECURE';
  return (
    <div className={`rounded-xl p-6 ${isSecure
      ? 'bg-emerald-50 border border-emerald-100'
      : 'bg-red-50 border border-red-100'}`}>
      <div className="flex items-center gap-4">
        <div className={`w-12 h-12 rounded-full flex items-center justify-center ${
          isSecure ? 'bg-emerald-100' : 'bg-red-100'
        }`}>
          {isSecure
            ? <ShieldCheck size={22} className="text-emerald-600" />
            : <ShieldAlert size={22} className="text-red-500" />}
        </div>
        <div>
          <div className={`text-xl font-bold tracking-tight ${isSecure ? 'text-emerald-700' : 'text-red-600'}`}>
            {isSecure ? 'Cryptographically Secure' : 'Potentially Weak'}
          </div>
          <div className="text-xs text-gray-500 mt-0.5">
            ML confidence: {(confidence * 100).toFixed(1)}%
          </div>
        </div>
        <div className="ml-auto text-right">
          <div className="text-[10px] font-bold text-gray-400 uppercase tracking-widest mb-1">Probability</div>
          <div className="flex gap-3">
            <div className="text-center">
              <div className="text-sm font-bold text-emerald-600">{(probSecure * 100).toFixed(1)}%</div>
              <div className="text-[9px] text-gray-400 uppercase tracking-wider">Secure</div>
            </div>
            <div className="text-center">
              <div className="text-sm font-bold text-red-500">{(probWeak * 100).toFixed(1)}%</div>
              <div className="text-[9px] text-gray-400 uppercase tracking-wider">Weak</div>
            </div>
          </div>
        </div>
      </div>

      {/* Probability bar */}
      <div className="mt-4">
        <div className="flex rounded-full overflow-hidden h-2">
          <div className="bg-emerald-400 transition-all" style={{ width: `${probSecure * 100}%` }} />
          <div className="bg-red-400 transition-all" style={{ width: `${probWeak * 100}%` }} />
        </div>
        <div className="flex justify-between text-[9px] text-gray-400 mt-1">
          <span>← Secure</span>
          <span>Weak →</span>
        </div>
      </div>
    </div>
  );
}

// ── Main Component ────────────────────────────────────────────────────────────
export default function SecurityAudit() {
  const { token } = useAuth();
  const [keys, setKeys] = useState([]);
  const [selectedKey, setSelectedKey] = useState('');
  const [result, setResult] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [serviceOnline, setServiceOnline] = useState(null);

  // Check if BitSecure service is running
  useEffect(() => {
    fetch('/bitsecure/health')
      .then(r => r.ok ? r.json() : null)
      .then(d => setServiceOnline(d?.status === 'ok'))
      .catch(() => setServiceOnline(false));
  }, []);

  // Load active keys
  useEffect(() => {
    apiFetch('/kms/listKeys', { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.ok ? r.json() : { keys: [] })
      .then(d => {
        const active = (d.keys || []).filter(k => k.status === 'active');
        setKeys(active);
        if (active.length > 0) setSelectedKey(active[0].key_id);
      });
  }, []);

  const runAnalysis = async () => {
    if (!selectedKey) return;
    setLoading(true);
    setError('');
    setResult(null);

    try {
      // Step 1: get key material from Go backend
      const matRes = await apiFetch(`/kms/keyMaterial?id=${encodeURIComponent(selectedKey)}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      const matData = await matRes.json();
      if (!matRes.ok) {
        setError(matData.error || 'Failed to fetch key material from backend.');
        return;
      }
      const { key_material, version } = matData;

      // Step 2: send to BitSecure analysis server
      let analysisRes;
      try {
        analysisRes = await fetch('/bitsecure/analyze', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ key_material, key_id: selectedKey }),
        });
      } catch {
        setError('Could not reach the BitSecure analysis service. Make sure it is running: python3 crypto/server.py');
        return;
      }
      const analysisData = await analysisRes.json();
      if (!analysisRes.ok) {
        setError(analysisData.error || 'Analysis failed');
        return;
      }
      setResult({ ...analysisData, version });
    } catch (e) {
      setError(`Unexpected error: ${e.message}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="p-8 max-w-5xl">
      <div className="flex items-center justify-between mb-2">
        <div>
          <h1 className="text-3xl font-bold text-gray-900 tracking-tight">Security Audit</h1>
          <p className="text-sm text-gray-500 mt-1">
            ML-powered randomness analysis using BitSecure — detects cryptographically weak key material.
          </p>
        </div>
        <div className={`flex items-center gap-2 text-[10px] font-bold uppercase tracking-widest px-3 py-1.5 rounded-full border ${
          serviceOnline === null ? 'border-gray-200 text-gray-400' :
          serviceOnline ? 'border-emerald-200 bg-emerald-50 text-emerald-700' :
          'border-red-200 bg-red-50 text-red-600'
        }`}>
          <span className={`w-1.5 h-1.5 rounded-full ${
            serviceOnline === null ? 'bg-gray-400' :
            serviceOnline ? 'bg-emerald-500 animate-pulse' : 'bg-red-500'
          }`} />
          {serviceOnline === null ? 'Checking...' : serviceOnline ? 'BitSecure Online' : 'BitSecure Offline'}
        </div>
      </div>

      {/* Service offline warning */}
      {serviceOnline === false && (
        <div className="mb-6 p-4 bg-amber-50 border border-amber-200 rounded-xl flex items-start gap-3">
          <AlertTriangle size={16} className="text-amber-500 mt-0.5 shrink-0" />
          <div>
            <div className="text-sm font-semibold text-amber-800 mb-1">BitSecure Analysis Service Not Running</div>
            <p className="text-xs text-amber-700 leading-relaxed">
              Start it with: <code className="bg-amber-100 px-1.5 py-0.5 rounded font-mono">python3 crypto/server.py</code>
            </p>
          </div>
        </div>
      )}

      {/* Key selector + run */}
      <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-6 mb-6">
        <div className="text-[11px] font-bold text-gray-500 tracking-widest uppercase mb-3">Select Key to Audit</div>
        <div className="flex gap-3">
          <div className="relative flex-1">
            <select
              value={selectedKey}
              onChange={e => setSelectedKey(e.target.value)}
              className="w-full appearance-none border border-gray-200 rounded-lg px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-200 focus:border-indigo-400 transition bg-white font-mono pr-8"
            >
              {keys.length === 0
                ? <option value="">No active keys available</option>
                : keys.map(k => <option key={k.key_id} value={k.key_id}>{k.key_id}</option>)
              }
            </select>
            <ChevronDown size={14} className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 pointer-events-none" />
          </div>
          <button
            onClick={runAnalysis}
            disabled={loading || !selectedKey || !serviceOnline}
            className="bg-indigo-600 hover:bg-indigo-700 text-white px-6 py-2.5 rounded-lg text-xs font-bold tracking-wider uppercase flex items-center gap-2 disabled:opacity-50 transition-colors shadow-sm shadow-indigo-600/20"
          >
            {loading
              ? <><RefreshCw size={13} className="animate-spin" /> Analyzing...</>
              : <><Activity size={13} /> Run Audit</>}
          </button>
        </div>
        <p className="text-[11px] text-gray-400 mt-2">
          The key's raw bytes are analyzed locally. Key material is never sent outside your network.
        </p>
      </div>

      {error && (
        <div className="mb-6 p-4 bg-red-50 border border-red-100 rounded-xl text-sm text-red-600 flex items-center gap-2">
          <XCircle size={15} className="shrink-0" /> {error}
        </div>
      )}

      {/* Results */}
      {result && (
        <div className="space-y-6">
          {/* Header info */}
          <div className="flex items-center gap-3 text-xs text-gray-500">
            <span className="font-mono font-semibold text-gray-800">{result.key_id}</span>
            <span>·</span>
            <span>v{result.version}</span>
            <span>·</span>
            <span>{result.key_length_bits} bits</span>
            <span>·</span>
            <span>Analyzed at {result.analysis_length_bits} bits</span>
            <span>·</span>
            <span>Random Forest model</span>
          </div>

          {/* Verdict */}
          <VerdictBadge
            verdict={result.verdict}
            confidence={result.confidence}
            probWeak={result.prob_weak}
            probSecure={result.prob_secure}
          />

          {/* 2-col grid */}
          <div className="grid grid-cols-2 gap-6">
            {/* Bit Heatmap */}
            <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-6">
              <BitHeatmap bits={result.heatmap_bits} />
            </div>

            {/* Byte Distribution */}
            <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-6">
              <ByteDistribution bytes={result.byte_distribution} />
              <div className="mt-5">
                <AutocorrChart profile={result.autocorr_profile} />
              </div>
            </div>

            {/* NIST Results */}
            <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-6">
              <NistResults nist={result.nist} />
            </div>

            {/* Feature Importance */}
            <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-6">
              <FeatureImportance data={result.feature_importance} />
            </div>
          </div>

          {/* Raw features table */}
          <div className="bg-white rounded-xl border border-gray-100 shadow-sm overflow-hidden">
            <div className="px-6 py-4 border-b border-gray-100">
              <div className="text-[10px] font-bold text-gray-400 tracking-widest uppercase">All 18 Statistical Features</div>
            </div>
            <div className="grid grid-cols-3 divide-x divide-gray-100">
              {Object.entries(result.features).map(([key, val]) => {
                const LABELS = {
                  bit_freq: 'Bit Frequency', num_runs: 'Run Count',
                  longest_run_0: 'Longest 0-Run', longest_run_1: 'Longest 1-Run',
                  autocorr_lag1: 'Autocorr Lag 1', autocorr_lag2: 'Autocorr Lag 2',
                  autocorr_lag3: 'Autocorr Lag 3', autocorr_lag4: 'Autocorr Lag 4',
                  autocorr_lag5: 'Autocorr Lag 5', approx_entropy: 'Approx Entropy',
                  byte_entropy: 'Byte Entropy', spectral_mean: 'Spectral Mean',
                  spectral_std: 'Spectral Std', spectral_max: 'Spectral Peak',
                  compression_ratio: 'Compression Ratio', ngram2_chi2: '2-gram χ²',
                  ngram3_chi2: '3-gram χ²', linear_complexity: 'Linear Complexity',
                };
                return (
                  <div key={key} className="px-5 py-3 hover:bg-gray-50 transition-colors">
                    <div className="text-[10px] text-gray-400 mb-0.5">{LABELS[key] || key}</div>
                    <div className="text-sm font-mono font-semibold text-gray-800">{Number(val).toFixed(4)}</div>
                  </div>
                );
              })}
            </div>
          </div>

          {/* Interpretation note */}
          <div className="p-4 bg-gray-50 border border-gray-100 rounded-xl flex items-start gap-3">
            <Info size={14} className="text-gray-400 mt-0.5 shrink-0" />
            <p className="text-[11px] text-gray-500 leading-relaxed">
              This analysis uses a Random Forest classifier trained on 320,000 bit sequences from 8 generators
              (5 weak, 3 secure). The model achieves ROC-AUC 0.975 at 16,384 bits and 0.958 at 256 bits.
              A "Potentially Weak" verdict at 256 bits has a higher false-positive rate — consider it a signal
              to investigate, not a definitive failure. All keys generated by this system use <code className="bg-gray-100 px-1 rounded">crypto/rand</code> which is cryptographically secure.
            </p>
          </div>
        </div>
      )}

      {/* Empty state */}
      {!result && !loading && !error && (
        <div className="bg-white rounded-xl border border-gray-100 shadow-sm p-12 text-center">
          <div className="w-14 h-14 rounded-full bg-indigo-50 flex items-center justify-center mx-auto mb-4">
            <Cpu size={24} className="text-indigo-400" />
          </div>
          <h3 className="text-base font-bold text-gray-800 mb-2">ML-Powered Key Audit</h3>
          <p className="text-sm text-gray-400 max-w-md mx-auto leading-relaxed">
            Select a key and run an audit to see a full randomness analysis — bit heatmap,
            NIST SP 800-22 test results, autocorrelation profile, and a Random Forest verdict
            comparing your key against known weak PRNGs.
          </p>
        </div>
      )}
    </div>
  );
}
