// Local cache to map node IDs (e.g. "node2") to addresses (e.g. "localhost:5002")
let clusterMapCache = null;

export async function apiFetch(url, options = {}) {
  let endpoint = url;
  
  // Try up to 3 times in case of leader elections or redirects
  for (let i = 0; i < 3; i++) {
    const response = await fetch(endpoint, options);
    
    // If successful (2xx), return immediately
    if (response.ok) {
      return response;
    }
    
    // If we hit a node that isn't the leader (503 Service Unavailable)
    if (response.status === 503) {
      const data = await response.clone().json().catch(() => null);
      if (data && data.error === 'not leader' && data.leader) {
        console.warn(`Node is not leader. Redirecting requested to leader ID: ${data.leader}`);
        
        // Load the map of node_id -> address if we haven't already
        // Wait, if clusterMapCache is empty, how do we query? 
        // We can query the base proxy `/cluster/status` which hits the node we are connected to.
        if (!clusterMapCache) {
          try {
            const statusRes = await fetch('/cluster/status');
            if (statusRes.ok) {
              const statusData = await statusRes.json();
              clusterMapCache = {};
              statusData.nodes.forEach(node => {
                if (node.node_id && node.address) {
                  clusterMapCache[node.node_id] = node.address;
                }
              });
            }
          } catch (e) {
            console.error("Failed to map cluster addresses:", e);
          }
        }

        const leaderAddr = clusterMapCache ? clusterMapCache[data.leader] : null;
        
        if (leaderAddr) {
          // Construct direct URL to the leader bypassing the proxy
          // Note: Backend server handles CORS, so this will succeed.
          endpoint = `http://${leaderAddr}${url}`;
          console.log(`Redirecting request to -> ${endpoint}`);
          continue; // Retry the loop with the new endpoint!
        }
      }
    }
    
    // Return standard response for other errors
    return response;
  }
  
  throw new Error('Too many redirects or cluster unstable');
}
