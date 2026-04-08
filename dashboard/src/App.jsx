import { useState, useEffect, useRef, useCallback } from 'react'
import './index.css'

const host = window.location.hostname || '127.0.0.1';
const DEFAULT_NODES = [`${host}:5001`, `${host}:5002`, `${host}:5003`];
const NODE_POSITIONS = {
  0: { cx: '50%', cy: '25%' },
  1: { cx: '25%', cy: '75%' },
  2: { cx: '75%', cy: '75%' },
  // Adding more positions dynamically in case of cluster expansion
  3: { cx: '50%', cy: '50%' },
  4: { cx: '25%', cy: '25%' },
}

function getEventCategory(type) {
  if (['ELECTION_START', 'LEADER_ELECTED'].includes(type)) return 'election'
  if (['VOTE_GRANTED', 'VOTE_REJECTED', 'REQUEST_VOTE'].includes(type)) return 'vote'
  if (['LOG_REPLICATED', 'APPEND_ENTRIES', 'HEARTBEAT'].includes(type)) return 'replication'
  if (['ENTRY_COMMITTED', 'ENTRY_APPLIED'].includes(type)) return 'commit'
  if (['CHAOS_KILL', 'CHAOS_REVIVE', 'CHAOS_DELAY', 'CHAOS_DROP', 'ROLE_CHANGE'].includes(type)) return 'chaos'
  if (type.includes('KMS') || ['KEY_CREATED', 'KEY_DELETED', 'KEY_ROTATED', 'ENCRYPT', 'DECRYPT'].includes(type)) return 'kms'
  return 'replication'
}

function formatTime(ts) {
  if (!ts) return ''
  const d = new Date(ts)
  return d.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit', fractionalSecondDigits: 1 })
}

function App() {
  // Auth state
  const [apiKey, setApiKey] = useState(localStorage.getItem('raft_api_key') || '')
  const [isAuthenticated, setIsAuthenticated] = useState(false)
  const [currentUser, setCurrentUser] = useState(null)

  // Cluster State
  const [nodes, setNodes] = useState([])
  const [events, setEvents] = useState([])
  const [keys, setKeys] = useState([])
  const [users, setUsers] = useState([])
  const [auditLog, setAuditLog] = useState([])
  const [raftLog, setRaftLog] = useState({ entries: [], commit_index: 0 })
  
  const [addressInput, setAddressInput] = useState(DEFAULT_NODES.join(', '))
  const [nodeAddresses, setNodeAddresses] = useState(DEFAULT_NODES)
  const [connected, setConnected] = useState(false)
  const [toasts, setToasts] = useState([])
  const [packets, setPackets] = useState([])

  // Operation States
  const [newKeyId, setNewKeyId] = useState('')
  const [encKeyId, setEncKeyId] = useState('')
  const [encPlaintext, setEncPlaintext] = useState('')
  const [encResult, setEncResult] = useState(null)
  
  // Org Admin States
  const [newUsername, setNewUsername] = useState('')
  const [newUserRole, setNewUserRole] = useState('service')
  const [newNodeAddr, setNewNodeAddr] = useState('')
  const [partSource, setPartSource] = useState('')
  const [partTarget, setPartTarget] = useState('')

  const eventsRef = useRef(null)
  const eventSourcesRef = useRef([])

  const leaderNode = nodes.find(n => n.role === 'LEADER' && n.is_alive && !n.is_chaos_killed)
  const leaderAddr = leaderNode?.address || nodeAddresses[0]

  const showToast = useCallback((msg, type = 'success') => {
    const id = Date.now() + Math.random()
    setToasts(prev => [...prev, { id, msg, type }])
    setTimeout(() => { setToasts(prev => prev.filter(t => t.id !== id)) }, 4000)
  }, [])

  const api = useCallback(async (addr, path, method = 'GET', body = null) => {
    const opts = { 
      method, 
      headers: { 'Content-Type': 'application/json' } 
    }
    if (apiKey) {
      opts.headers['Authorization'] = `Bearer ${apiKey}`
    }
    if (body) opts.body = JSON.stringify(body)
    
    const res = await fetch(`http://${addr}${path}`, opts)
    const json = await res.json()
    if (!res.ok) throw new Error(json.error || 'API Error')
    return json
  }, [apiKey])

  const firePacket = useCallback((fromId, toId, type) => {
    if (!fromId || !toId) return
    const id = Date.now() + Math.random()
    setPackets(prev => [...prev, { id, from: fromId, to: toId, type }])
    setTimeout(() => { setPackets(prev => prev.filter(p => p.id !== id)) }, 800)
  }, [])

  // Login handler
  const handleLogin = async (e) => {
    e.preventDefault()
    if (!apiKey) return
    try {
      // Test key by fetching users (requires auth)
      const data = await api(nodeAddresses[0], '/kms/listUsers')
      setIsAuthenticated(true)
      localStorage.setItem('raft_api_key', apiKey)
      showToast('Authenticated securely')
      
      // Auto-detect current user from the list
      // In a real app the API would return /me, but we map here for demo
      if (data.users) {
        setUsers(data.users)
        const me = data.users.find(u => u.api_key === apiKey)
        if (me) setCurrentUser(me)
      }
    } catch (e) {
      showToast('Authentication failed: ' + e.message, 'error')
    }
  }

  const handleLogout = () => {
    setApiKey('')
    setIsAuthenticated(false)
    setCurrentUser(null)
    localStorage.removeItem('raft_api_key')
  }

  // SSE Connect
  const connectSSE = useCallback(() => {
    if (!isAuthenticated) return
    eventSourcesRef.current.forEach(es => es.close())
    eventSourcesRef.current = []

    nodeAddresses.forEach(addr => {
      try {
        const es = new EventSource(`http://${addr}/events`)
        es.onmessage = (e) => {
          try {
            const event = JSON.parse(e.data)
            if (event.type === 'CONNECTED') return
            setEvents(prev => [...prev.slice(-199), event])
            
            if (event.type === 'APPEND_ENTRIES') {
              firePacket(event.details?.from, event.node_id, 'append')
            } else if (event.type === 'REQUEST_VOTE') {
              firePacket(event.details?.candidate, event.node_id, 'vote')
            }
          } catch {}
        }
        eventSourcesRef.current.push(es)
      } catch {}
    })
    setConnected(true)
  }, [nodeAddresses, firePacket, isAuthenticated])

  // Long Polling for cluster state
  useEffect(() => {
    if (!connected || !isAuthenticated) return
    const poll = async () => {
      for (const addr of nodeAddresses) {
        try {
          const data = await api(addr, '/cluster/status')
          if (data.nodes) { 
            setNodes(data.nodes)
            // Dynamically add new addresses if cluster expands
            const newAddrs = data.nodes.map(n => n.address)
            if (newAddrs.length > nodeAddresses.length) {
              setNodeAddresses(newAddrs)
              setAddressInput(newAddrs.join(', '))
            }
            break 
          }
        } catch {}
      }
      for (const addr of nodeAddresses) {
        try {
          const data = await api(addr, '/kms/listKeys')
          if (data.keys) { setKeys(data.keys || []); break }
        } catch {}
      }
      for (const addr of nodeAddresses) {
        try {
          const data = await api(addr, '/kms/listUsers')
          if (data.users) { 
            setUsers(data.users)
            const me = data.users.find(u => u.api_key === apiKey)
            if (me) setCurrentUser(me)
            break 
          }
        } catch {}
      }
      for (const addr of nodeAddresses) {
        try {
          const data = await api(addr, '/kms/auditLog')
          if (data.audit_trail) { setAuditLog(data.audit_trail); break }
        } catch {}
      }
      for (const addr of nodeAddresses) {
        try {
          const data = await api(addr, '/raft/log')
          if (data.entries !== undefined) { setRaftLog(data); break }
        } catch {}
      }
    }
    poll()
    const interval = setInterval(poll, 1500)
    return () => clearInterval(interval)
  }, [connected, nodeAddresses, api, isAuthenticated, apiKey])

  useEffect(() => {
    if (isAuthenticated) {
      connectSSE()
    }
    return () => eventSourcesRef.current.forEach(es => es.close())
  }, [connectSSE, isAuthenticated])

  useEffect(() => {
    if (eventsRef.current) eventsRef.current.scrollTop = eventsRef.current.scrollHeight
  }, [events])

  const handleConnect = () => {
    const addrs = addressInput.split(',').map(s => s.trim()).filter(Boolean)
    if (addrs.length >= 3) {
      setNodeAddresses(addrs)
      setEvents([])
      setTimeout(() => connectSSE(), 100)
      showToast('Connecting to mapped addresses...')
    } else {
      showToast('Need at least 3 initial addresses separated by commas', 'error')
    }
  }

  // --- API Handlers ---
  
  const handleCreateUser = async () => {
    try {
      const data = await api(leaderAddr, '/kms/createUser', 'POST', { username: newUsername, role: newUserRole })
      showToast(`User constructed. API Key: ${data.api_key}`)
      setNewUsername('')
    } catch (e) { showToast(e.message, 'error') }
  }

  const handleDeleteUser = async (username) => {
    try {
      await api(leaderAddr, '/kms/deleteUser', 'POST', { username })
      showToast(`User ${username} deleted`)
    } catch (e) { showToast(e.message, 'error') }
  }

  const handleCreateKey = async () => {
    try {
      await api(leaderAddr, '/kms/createKey', 'POST', { key_id: newKeyId })
      showToast(`Key "${newKeyId}" created`)
      setNewKeyId('')
    } catch (e) { showToast(e.message, 'error') }
  }

  const handleEncrypt = async () => {
    try {
      const data = await api(leaderAddr, '/kms/encrypt', 'POST', { key_id: encKeyId, plaintext: encPlaintext })
      setEncResult({ type: 'success', data: data.ciphertext })
      showToast(`Encrypted securely and added to org audit trail.`)
    } catch (e) { setEncResult({ type: 'error', data: e.message }) }
  }

  const handleAddNode = async () => {
    try {
      await api(leaderAddr, '/cluster/addNode', 'POST', { address: newNodeAddr })
      showToast(`P2P AddNode command dispatched. Approving ${newNodeAddr}...`)
      setNewNodeAddr('')
    } catch (e) { showToast(e.message, 'error') }
  }

  const handlePartition = async () => {
    try {
      const sourceAddr = nodes.find(n => n.node_id === partSource)?.address;
      if (!sourceAddr) return;
      await api(sourceAddr, '/chaos/partition', 'POST', { target: partTarget });
      showToast(`Node ${partSource} isolated from ${partTarget}`, 'error');
    } catch (e) { showToast(e.message, 'error') }
  }

  const handleHeal = async () => {
    try {
      const sourceAddr = nodes.find(n => n.node_id === partSource)?.address;
      if (!sourceAddr) return;
      await api(sourceAddr, '/chaos/heal', 'POST', { target: partTarget });
      showToast(`Healed partition ${partSource} 🔗 ${partTarget}`);
    } catch (e) { showToast(e.message, 'error') }
  }

  const getNodePos = (nodeId) => {
    const sortedNodes = [...nodes].sort((a,b) => a.node_id.localeCompare(b.node_id))
    const idx = sortedNodes.findIndex(n => n.node_id === nodeId)
    return NODE_POSITIONS[idx] || { cx: `${10 + (idx*15)}%`, cy: `${10 + (idx*10)}%` }
  }

  // --- LOGIN RENDER ---
  if (!isAuthenticated) {
    return (
      <div className="app login-view">
        <div className="glass-panel login-card">
          <div style={{textAlign: 'center', marginBottom: 24}}>
            <h1 className="header-logo" style={{fontSize: 28, marginBottom: 8}}>⚡ RaftKMS</h1>
            <p style={{color: 'var(--text-secondary)', fontSize: 13}}>Enterprise Org Security Console</p>
          </div>
          <form onSubmit={handleLogin}>
            <div className="form-group">
              <label className="form-label">Cluster Root Node (Initial Contact)</label>
              <input className="input-glass" style={{width: '100%', marginBottom: 16}} value={addressInput} onChange={e=>setAddressInput(e.target.value)} />
              
              <label className="form-label">API Access Key</label>
              <input type="password" className="input-glass" style={{width: '100%', marginBottom: 24}} placeholder="Enter your identity key..." value={apiKey} onChange={e=>setApiKey(e.target.value)} />
              
              <button type="submit" className="btn btn-primary" style={{width: '100%', justifyContent: 'center', padding: '10px'}}>Authenticate</button>
            </div>
            <div style={{fontSize: 10, color: 'var(--text-muted)', textAlign: 'center', marginTop: 16}}>
              Default admin key is normally printed to terminal on bootstrap: <code style={{color:'var(--neon-green)'}}>admin-secret-key</code>
            </div>
          </form>
        </div>
        <div className="toast-container">
          {toasts.map(t => <div key={t.id} className={`toast ${t.type}`}>{t.msg}</div>)}
        </div>
      </div>
    )
  }

  const isAdmin = currentUser?.role === 'admin'

  // --- MAIN APP RENDER ---
  return (
    <div className="app">
      <header className="header">
        <div style={{display:'flex', alignItems:'center'}}>
          <div className="header-logo">⚡ RaftKMS</div>
          <span className="header-badge">ORG CONSOLE</span>
          <span className={`status-dot ${connected ? 'connected' : 'disconnected'}`} style={{marginLeft: 12}}></span>
        </div>
        <div className="connection-bar">
          <div style={{fontSize: 12, color: 'var(--text-secondary)', marginRight: 16}}>
            Identity: <span style={{color: isAdmin ? 'var(--neon-red)' : 'var(--neon-blue)', fontWeight: 600}}>@{currentUser?.username || 'unknown'}</span> ({currentUser?.role || 'readonly'})
          </div>
          <button className="btn" onClick={handleLogout}>Logout</button>
        </div>
      </header>

      <div className="main-layout enterprise-grid">
        
        {/* TOP LEFT: COMPLIANCE AUDIT TRAIL */}
        <div className="glass-panel panel-audit">
          <div className="panel-header">
            <span>🛡️ Cryptographic Audit Ledger</span>
            <span style={{color:'var(--neon-blue)'}}>{auditLog.length} Records</span>
          </div>
          <div className="panel-body timeline-list" style={{background: 'rgba(0,0,0,0.5)'}}>
            {auditLog.length === 0 && <div className="event-details" style={{textAlign:'center',marginTop:20}}>No cryptographic operations logged yet.</div>}
            {auditLog.slice().reverse().map((entry, i) => (
              <div key={i} className="timeline-item" style={{borderLeft: `2px solid ${entry.action==='ENCRYPT' ? 'var(--neon-green)' : 'var(--neon-orange)'}`}}>
                <div className="time-col" style={{width: 50}}>{new Date(entry.timestamp).toLocaleTimeString([], {hour:'2-digit', minute:'2-digit', second:'2-digit'})}</div>
                <div className="event-details">
                  <span style={{color: 'var(--neon-purple)', fontWeight: 600}}>@{entry.username}</span> 
                  {' performed '}
                  <span style={{color: entry.action==='ENCRYPT'?'var(--neon-green)':'var(--neon-orange)', fontWeight: 600}}>{entry.action}</span>
                  {' on Key '}
                  <span style={{color: 'var(--text-primary)'}}>{entry.key_id}</span>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* BOTTOM LEFT: P2P MEMBERSHIP & USERS */}
        <div className="glass-panel panel-users">
          <div className="panel-header">
            <span>👥 Org Identity & Membership</span>
          </div>
          <div className="panel-body">
            {isAdmin ? (
              <>
                <h4 className="form-label" style={{color:'var(--neon-cyan)'}}>IAM Provisioning (Admin)</h4>
                <div className="form-group" style={{display:'flex',gap:8}}>
                  <input className="input-glass" style={{flex:1, width: 80}} placeholder="Username" value={newUsername} onChange={e=>setNewUsername(e.target.value)} />
                  <select className="input-glass" style={{width: 80}} value={newUserRole} onChange={e=>setNewUserRole(e.target.value)}>
                    <option value="service">Service</option>
                    <option value="admin">Admin</option>
                  </select>
                  <button className="btn btn-primary" onClick={handleCreateUser}>Add</button>
                </div>
              </>
            ) : (
              <div style={{fontSize: 11, color: 'var(--neon-orange)', marginBottom: 12}}>IAM restricted to Admins.</div>
            )}
            
            <table className="log-table" style={{marginTop: 8, marginBottom: 20}}>
              <thead>
                <tr><th>User</th><th>Role</th><th>Registered</th>{isAdmin && <th>Action</th>}</tr>
              </thead>
              <tbody>
                {users.map(u => (
                  <tr key={u.username}>
                    <td style={{color: 'var(--neon-purple)'}}>@{u.username}</td>
                    <td>{u.role === 'admin' ? '👑 Admin' : '⚙️ Service'}</td>
                    <td>{new Date(u.created_at).toLocaleDateString()}</td>
                    {isAdmin && <td>
                      {u.username !== 'admin' && <button className="btn btn-danger" style={{padding:'2px 6px', fontSize: 9}} onClick={()=>handleDeleteUser(u.username)}>Del</button>}
                    </td>}
                  </tr>
                ))}
              </tbody>
            </table>

            {isAdmin && (
              <>
                <h4 className="form-label" style={{color:'var(--neon-red)', marginTop: 24}}>Cluster Dynamic Config (P2P Mesh)</h4>
                <div className="form-group" style={{display:'flex',gap:8}}>
                  <input className="input-glass" style={{flex:1, width: 80}} placeholder="IP or host:port" value={newNodeAddr} onChange={e=>setNewNodeAddr(e.target.value)} />
                  <button className="btn btn-primary" onClick={handleAddNode}>Join Node</button>
                </div>
              </>
            )}
          </div>
        </div>

        {/* CENTER OVERLAY: SVG NETWORK TOPOLOGY */}
        <div className="topology-container">
          <svg className="svg-network">
            <defs>
              <filter id="glow" x="-20%" y="-20%" width="140%" height="140%"><feGaussianBlur stdDeviation="3" result="blur" /><feComposite in="SourceGraphic" in2="blur" operator="over" /></filter>
            </defs>
            {nodes.map((n1, i) => 
               nodes.map((n2, j) => {
                 if (i >= j) return null;
                 const p1 = getNodePos(n1.node_id);
                 const p2 = getNodePos(n2.node_id);
                 const isPartitioned = n1.partitions?.includes(n2.node_id) || n2.partitions?.includes(n1.node_id);
                 return <line key={`line-${i}-${j}`} x1={p1.cx} y1={p1.cy} x2={p2.cx} y2={p2.cy} className={`network-line ${isPartitioned ? 'partitioned' : 'active'}`} />
               })
            )}
            {packets.map(p => {
              const p1 = getNodePos(p.from);
              const p2 = getNodePos(p.to);
              return (
                <circle key={p.id} r="4" className={`packet ${p.type}`} filter="url(#glow)">
                  <animate attributeName="cx" values={`${p1.cx};${p2.cx}`} dur="0.6s" fill="freeze" />
                  <animate attributeName="cy" values={`${p1.cy};${p2.cy}`} dur="0.6s" fill="freeze" />
                  <animate attributeName="opacity" values="1;1;0" keyTimes="0;0.8;1" dur="0.6s" fill="freeze" />
                </circle>
              )
            })}
          </svg>
          <div className="node-ui-layer">
            {nodes.map(node => {
              const pos = getNodePos(node.node_id);
              const roleClass = (!node.is_alive || node.is_chaos_killed) ? 'offline' : node.role.toLowerCase();
              return (
                <div key={node.node_id} className={`node-orb ${roleClass}`} style={{left: pos.cx, top: pos.cy}}>
                  <div className="orb-id">{node.node_id}</div>
                  <div className="orb-role">{roleClass.toUpperCase()}</div>
                  <div className="orb-stats">
                    Term: <span>{node.current_term}</span>
                    Log: <span>{node.log_length}</span>
                  </div>
                </div>
              )
            })}
          </div>
        </div>

        {/* RIGHT PANEL: CRYPTO OPERATIONS */}
        <div className="glass-panel panel-right">
          <div className="panel-header">
            <span>🔒 Cryptographic Workspace</span>
            {leaderNode && <span style={{color:'var(--neon-green)'}}>Leader: {leaderNode.node_id}</span>}
          </div>
          <div className="panel-body">
            
            <h4 className="form-label" style={{color:'var(--neon-purple)'}}>Available Org Keys</h4>
            {isAdmin && <div className="form-group" style={{display:'flex',gap:8}}>
              <input className="input-glass" style={{flex:1,width:'auto'}} placeholder="New Key ID" value={newKeyId} onChange={e=>setNewKeyId(e.target.value)} />
              <button className="btn btn-primary" onClick={handleCreateKey}>Generate</button>
            </div>}
            <div className="form-group" style={{marginBottom: 24}}>
              {keys.map(k => (
                <div key={k.key_id} className="key-list-item">
                  <span style={{fontWeight:600}}>{k.key_id} <span style={{color:'var(--text-muted)'}}>(v{k.versions?.length||1})</span></span>
                  {isAdmin && k.status === 'active' && <button className="btn btn-danger" style={{padding:'2px 6px', fontSize: 9}} onClick={()=>api(leaderAddr, '/kms/deleteKey', 'POST', {key_id: k.key_id})}>Del</button>}
                </div>
              ))}
              {keys.length === 0 && <div style={{fontSize:11, color:'var(--text-muted)'}}>No active keys. Admins must generate a key first.</div>}
            </div>

            <h4 className="form-label" style={{color:'var(--neon-cyan)'}}>Data Encryption (AES-GCM)</h4>
            <div className="form-group" style={{marginBottom: 24}}>
              <select className="input-glass" style={{width:'100%', marginBottom:8}} value={encKeyId} onChange={e=>setEncKeyId(e.target.value)}>
                <option value="">Select Key...</option>
                {keys.map(k => <option key={k.key_id} value={k.key_id}>{k.key_id}</option>)}
              </select>
              <textarea className="input-glass" style={{width:'100%', minHeight: 60, marginBottom:8, resize:'vertical'}} placeholder="Plaintext to encrypt..." value={encPlaintext} onChange={e=>setEncPlaintext(e.target.value)} />
              <button className="btn btn-success" style={{width:'100%', justifyContent: 'center'}} onClick={handleEncrypt}>Encrypt & Audit</button>
              {encResult && <div style={{fontSize:10,marginTop:6,wordBreak:'break-all',padding:8,background:'rgba(0,0,0,0.3)',borderRadius:6,color:encResult.type==='error'?'var(--neon-red)':'var(--neon-green)'}}>{encResult.data}</div>}
            </div>

            {isAdmin && (
              <div style={{marginTop: 60}}>
                <h4 className="form-label" style={{color:'var(--neon-orange)'}}>Split-Brain Simulator (Networking)</h4>
                <div className="form-group" style={{display:'flex',gap:8}}>
                  <select className="input-glass" style={{flex:1,padding:'4px'}} value={partSource} onChange={e=>setPartSource(e.target.value)}>
                    <option value="">Src</option>
                    {nodes.map(n => <option key={n.node_id} value={n.node_id}>{n.node_id}</option>)}
                  </select>
                  <select className="input-glass" style={{flex:1,padding:'4px'}} value={partTarget} onChange={e=>setPartTarget(e.target.value)}>
                    <option value="">Tgt</option>
                    {nodes.map(n => <option key={n.node_id} value={n.node_id}>{n.node_id}</option>)}
                  </select>
                </div>
                <div style={{display:'flex',gap:8}}>
                  <button className="btn btn-danger" style={{flex:1}} onClick={handlePartition}>✂️ Drop Pkt</button>
                  <button className="btn btn-primary" style={{flex:1}} onClick={handleHeal}>🔗 Restore</button>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>

      <div className="toast-container">
        {toasts.map(t => <div key={t.id} className={`toast ${t.type}`}>{t.msg}</div>)}
      </div>
    </div>
  )
}

export default App
