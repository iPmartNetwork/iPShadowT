package web

// fullDashboardHTML is the complete real-time dashboard with charts and WebSocket
const fullDashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>iPShadowT Dashboard</title>
<style>
:root{--bg:#0f172a;--card:#1e293b;--border:#334155;--text:#e2e8f0;--muted:#94a3b8;--accent:#38bdf8;--green:#34d399;--red:#f87171;--yellow:#fbbf24;--purple:#a78bfa}
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:var(--bg);color:var(--text);min-height:100vh}
.container{max-width:1400px;margin:0 auto;padding:20px}
header{display:flex;justify-content:space-between;align-items:center;margin-bottom:24px;padding-bottom:16px;border-bottom:1px solid var(--border)}
header h1{font-size:22px;color:var(--accent);display:flex;align-items:center;gap:10px}
header .status{display:flex;align-items:center;gap:8px;font-size:14px}
header .dot{width:8px;height:8px;border-radius:50%;background:var(--green);animation:pulse 2s infinite}
@keyframes pulse{0%,100%{opacity:1}50%{opacity:.5}}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:16px;margin-bottom:24px}
.card{background:var(--card);border:1px solid var(--border);border-radius:12px;padding:20px}
.card h3{font-size:12px;text-transform:uppercase;color:var(--muted);margin-bottom:12px;letter-spacing:.5px}
.stat-value{font-size:32px;font-weight:700;color:var(--accent)}
.stat-label{font-size:13px;color:var(--muted);margin-top:4px}
.stat-row{display:flex;justify-content:space-between;align-items:flex-end}
.chart-container{position:relative;height:200px;margin-top:12px}
canvas{width:100%!important;height:100%!important}
.wide{grid-column:span 2}
@media(max-width:768px){.wide{grid-column:span 1}}
table{width:100%;border-collapse:collapse;margin-top:12px}
th,td{padding:10px 12px;text-align:left;border-bottom:1px solid var(--border);font-size:13px}
th{color:var(--muted);font-size:11px;text-transform:uppercase}
.badge{padding:3px 8px;border-radius:4px;font-size:11px;font-weight:600}
.badge-green{background:#064e3b;color:var(--green)}
.badge-red{background:#450a0a;color:var(--red)}
.badge-yellow{background:#422006;color:var(--yellow)}
.badge-purple{background:#2e1065;color:var(--purple)}
.paths-list{display:flex;flex-direction:column;gap:8px}
.path-item{display:flex;justify-content:space-between;align-items:center;padding:10px 14px;background:var(--bg);border-radius:8px;border:1px solid var(--border)}
.path-item.active{border-color:var(--green)}
.path-name{font-weight:600;font-size:14px}
.path-info{font-size:12px;color:var(--muted)}
.tabs{display:flex;gap:4px;margin-bottom:16px}
.tab{padding:8px 16px;border-radius:6px;cursor:pointer;font-size:13px;background:transparent;border:1px solid var(--border);color:var(--muted);transition:all .2s}
.tab.active{background:var(--accent);color:var(--bg);border-color:var(--accent)}
.footer{text-align:center;padding:20px;color:var(--muted);font-size:12px;border-top:1px solid var(--border);margin-top:24px}
</style>
</head>
<body>
<div class="container">
<header>
<h1>&#128737; iPShadowT Dashboard</h1>
<div class="status"><div class="dot"></div><span id="conn-status">Connecting...</span></div>
</header>

<div class="grid">
<div class="card">
<h3>Active Connections</h3>
<div class="stat-value" id="active-conn">0</div>
<div class="stat-label">Total: <span id="total-conn">0</span></div>
</div>
<div class="card">
<h3>Traffic (Download)</h3>
<div class="stat-value" id="bytes-in">0 B</div>
<div class="stat-label">Rate: <span id="rate-in">0 B/s</span></div>
</div>
<div class="card">
<h3>Traffic (Upload)</h3>
<div class="stat-value" id="bytes-out">0 B</div>
<div class="stat-label">Rate: <span id="rate-out">0 B/s</span></div>
</div>
<div class="card">
<h3>Uptime</h3>
<div class="stat-value" id="uptime">0s</div>
<div class="stat-label">Users online: <span id="users-online">0</span></div>
</div>
</div>

<div class="grid">
<div class="card wide">
<h3>Traffic Chart (Real-time)</h3>
<div class="chart-container"><canvas id="traffic-chart"></canvas></div>
</div>
<div class="card">
<h3>Paths / Failover</h3>
<div class="paths-list" id="paths-list">
<div class="path-item active"><div><div class="path-name">Primary</div><div class="path-info">Waiting for data...</div></div><span class="badge badge-green">Active</span></div>
</div>
</div>
</div>

<div class="card">
<h3>Connected Users</h3>
<table>
<thead><tr><th>Name</th><th>Upload</th><th>Download</th><th>Status</th></tr></thead>
<tbody id="users-table"><tr><td colspan="4" style="color:var(--muted)">Loading...</td></tr></tbody>
</table>
</div>

<div class="footer">iPShadowT v1.0.0 &mdash; iPmart Network (Ali Hassanzadeh)</div>
</div>

<script>
const maxPoints=60;
let trafficData={in:[],out:[],labels:[]};
let ws;

function connectWS(){
const proto=location.protocol==='https:'?'wss:':'ws:';
ws=new WebSocket(proto+'//'+location.host+'/ws');
ws.onopen=()=>{document.getElementById('conn-status').textContent='Connected';};
ws.onclose=()=>{document.getElementById('conn-status').textContent='Reconnecting...';setTimeout(connectWS,3000);};
ws.onmessage=(e)=>{
try{const d=JSON.parse(e.data);updateDashboard(d);}catch(err){}
};
}

function updateDashboard(d){
document.getElementById('active-conn').textContent=d.active_connections||0;
document.getElementById('total-conn').textContent=d.total_connections||0;
document.getElementById('bytes-in').textContent=formatBytes(d.bytes_in||0);
document.getElementById('bytes-out').textContent=formatBytes(d.bytes_out||0);
document.getElementById('rate-in').textContent=formatBytes(d.bytes_in_rate||0)+'/s';
document.getElementById('rate-out').textContent=formatBytes(d.bytes_out_rate||0)+'/s';
document.getElementById('uptime').textContent=formatUptime(d.uptime_seconds||0);
document.getElementById('users-online').textContent=d.users_online||0;

// Update chart
trafficData.in.push(d.bytes_in_rate||0);
trafficData.out.push(d.bytes_out_rate||0);
trafficData.labels.push('');
if(trafficData.in.length>maxPoints){trafficData.in.shift();trafficData.out.shift();trafficData.labels.shift();}
drawChart();

if(d.active_path){
document.getElementById('paths-list').innerHTML='<div class="path-item active"><div><div class="path-name">'+d.active_path+'</div><div class="path-info">Paths: '+d.path_count+'</div></div><span class="badge badge-green">Active</span></div>';
}
}

function drawChart(){
const canvas=document.getElementById('traffic-chart');
const ctx=canvas.getContext('2d');
const w=canvas.parentElement.clientWidth;
const h=200;
canvas.width=w*2;canvas.height=h*2;
ctx.scale(2,2);
ctx.clearRect(0,0,w,h);

const maxVal=Math.max(...trafficData.in,...trafficData.out,1);
const stepX=w/(maxPoints-1);

// Draw grid
ctx.strokeStyle='#334155';ctx.lineWidth=0.5;
for(let i=0;i<5;i++){const y=h*i/4;ctx.beginPath();ctx.moveTo(0,y);ctx.lineTo(w,y);ctx.stroke();}

// Draw download line
ctx.beginPath();ctx.strokeStyle='#38bdf8';ctx.lineWidth=2;
trafficData.in.forEach((v,i)=>{const x=i*stepX;const y=h-(v/maxVal)*h*0.9;i===0?ctx.moveTo(x,y):ctx.lineTo(x,y);});
ctx.stroke();

// Draw upload line
ctx.beginPath();ctx.strokeStyle='#a78bfa';ctx.lineWidth=2;
trafficData.out.forEach((v,i)=>{const x=i*stepX;const y=h-(v/maxVal)*h*0.9;i===0?ctx.moveTo(x,y):ctx.lineTo(x,y);});
ctx.stroke();

// Legend
ctx.font='11px sans-serif';
ctx.fillStyle='#38bdf8';ctx.fillRect(w-120,8,12,3);ctx.fillText('Download',w-104,12);
ctx.fillStyle='#a78bfa';ctx.fillRect(w-120,20,12,3);ctx.fillText('Upload',w-104,24);
}

function formatBytes(b){
if(b>=1073741824)return(b/1073741824).toFixed(2)+' GB';
if(b>=1048576)return(b/1048576).toFixed(2)+' MB';
if(b>=1024)return(b/1024).toFixed(1)+' KB';
return Math.round(b)+' B';
}

function formatUptime(s){
const d=Math.floor(s/86400);const h=Math.floor((s%86400)/3600);const m=Math.floor((s%3600)/60);
if(d>0)return d+'d '+h+'h';
if(h>0)return h+'h '+m+'m';
return m+'m '+Math.floor(s%60)+'s';
}

connectWS();
setInterval(()=>{fetch('/api/dashboard/users').then(r=>r.json()).then(data=>{
if(Array.isArray(data)){
let html='';
data.forEach(u=>{
const badge=u.enabled?'<span class="badge badge-green">Active</span>':'<span class="badge badge-red">Disabled</span>';
html+='<tr><td>'+u.name+'</td><td>'+formatBytes(u.bytes_up)+'</td><td>'+formatBytes(u.bytes_down)+'</td><td>'+badge+'</td></tr>';
});
document.getElementById('users-table').innerHTML=html||'<tr><td colspan="4">No users</td></tr>';
}
}).catch(()=>{});},5000);

window.addEventListener('resize',drawChart);
</script>
</body>
</html>`
