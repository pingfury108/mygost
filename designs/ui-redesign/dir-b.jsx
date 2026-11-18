// 方向 B「曜石」：深色工具风，四屏
function BMainOn() {
  return (
    <div className="ab ab-b">
      <div className="top">
        <div className="brand mono" style={{ fontSize: 15, letterSpacing: '.14em' }}>MYGOST</div>
        <div className="pill"><span className="dot dot-grn"></span><span className="mono">ONLINE · 02:41:05</span></div>
      </div>

      <div className="center">
        <div className="big big-stop mono" style={{ fontWeight: 700, letterSpacing: '.06em' }}>断开</div>
        <div className="cur">
          <div className="cur-name mono">hk</div>
          <div className="cur-sub micro" style={{ marginTop: 5 }}>SSHD RELAY · 149.129.41.181:8081</div>
        </div>
      </div>

      <div className="tiles">
        <div className="tile">
          <div className="micro">上传 · UP</div>
          <div className="tv mono" style={{ color: 'var(--acc)' }}>1.2 MB/s</div>
          <div className="meter"><i style={{ width: '38%', background: 'var(--acc)' }}></i></div>
        </div>
        <div className="tile">
          <div className="micro">下载 · DOWN</div>
          <div className="tv mono" style={{ color: 'var(--grn)' }}>8.4 MB/s</div>
          <div className="meter"><i style={{ width: '76%', background: 'var(--grn)' }}></i></div>
        </div>
      </div>
      <div className="sessbar mono">会话 02:41:05　·　↑ 142 MB　·　↓ 1.2 GB</div>

      <div className="sec">
        <div className="sec-t micro">NODES · 配置</div>
        <div className="sec-a">＋ 添加</div>
      </div>
      <div className="list" style={{ borderWidth: 1, borderStyle: 'solid', borderColor: 'var(--line)' }}>
        <div className="row sel">
          <div className="chk" style={{ borderRadius: 6 }}>✓</div>
          <div className="rgrow">
            <div className="rname mono">hk　<span className="tag micro">PROXY</span></div>
            <div className="rsub micro">:8026 · HTTP</div>
          </div>
          <div className="more">⋯</div>
        </div>
        <div className="row">
          <div className="rgrow">
            <div className="rname mono">nas　<span className="tag micro">FORWARD</span></div>
            <div className="rsub micro mono">:2222 → 192.168.1.10:22</div>
          </div>
          <div className="more">⋯</div>
        </div>
      </div>

      <div className="foot mono"><span className="dot dot-grn"></span>SPLIT · CN DIRECT · 插件运行中</div>
    </div>
  );
}

function BMainOff() {
  return (
    <div className="ab ab-b">
      <div className="top">
        <div className="brand mono" style={{ fontSize: 15, letterSpacing: '.14em' }}>MYGOST</div>
        <div className="pill"><span className="dot dot-mut"></span><span className="mono">OFFLINE</span></div>
      </div>

      <div className="center">
        <div className="big big-go mono" style={{ fontWeight: 700, letterSpacing: '.06em' }}>连接</div>
        <div className="cur">
          <div className="cur-name mono">hk</div>
          <div className="cur-sub">将连接当前选中的配置</div>
        </div>
      </div>

      <div className="tiles">
        <div className="tile">
          <div className="micro">上传 · UP</div>
          <div className="tv mono" style={{ color: 'var(--tx2)' }}>—</div>
          <div className="meter"><i style={{ width: '0%' }}></i></div>
        </div>
        <div className="tile">
          <div className="micro">下载 · DOWN</div>
          <div className="tv mono" style={{ color: 'var(--tx2)' }}>—</div>
          <div className="meter"><i style={{ width: '0%' }}></i></div>
        </div>
      </div>
      <div className="sessbar mono"><span className="dot dot-grn"></span>分流规则已就绪 · 110,177 域名 / 9,662 IP</div>

      <div className="sec">
        <div className="sec-t micro">NODES · 配置</div>
        <div className="sec-a">＋ 添加</div>
      </div>
      <div className="list" style={{ borderWidth: 1, borderStyle: 'solid', borderColor: 'var(--line)' }}>
        <div className="row sel">
          <div className="chk" style={{ borderRadius: 6 }}>✓</div>
          <div className="rgrow">
            <div className="rname mono">hk　<span className="tag micro">PROXY</span></div>
            <div className="rsub micro">:8026 · HTTP</div>
          </div>
          <div className="more">⋯</div>
        </div>
        <div className="row">
          <div className="rgrow">
            <div className="rname mono">nas　<span className="tag micro">FORWARD</span></div>
            <div className="rsub micro mono">:2222 → 192.168.1.10:22</div>
          </div>
          <div className="more">⋯</div>
        </div>
      </div>

      <div className="foot">连接后可查看实时速率与流量</div>
    </div>
  );
}

function BSettings() {
  return (
    <div className="ab ab-b">
      <div className="subhead">
        <div className="back">←</div>
        <div className="stitle mono" style={{ letterSpacing: '.04em' }}>设置</div>
      </div>

      <div className="shead micro">SPLIT · 分流</div>
      <div className="card" style={{ borderWidth: 1, borderStyle: 'solid', borderColor: 'var(--line)' }}>
        <div className="srow">
          <div className="scol">
            <div className="stit">国内直连</div>
            <div className="ssub">国内域名与 IP 直连，其余走隧道</div>
          </div>
          <div className="sw on"><i></i></div>
        </div>
        <div className="srow">
          <div className="scol">
            <div className="stit mono" style={{ fontSize: 12.5 }}><span className="dot dot-grn" style={{ display: 'inline-block', marginRight: 6 }}></span>插件运行中</div>
            <div className="ssub mono">127.0.0.1:8100 · 9,662 IP / 110,177 域名</div>
          </div>
        </div>
        <div className="srow">
          <div className="scol">
            <div className="stit">域名列表</div>
            <div className="ssub mono">110,177 条 · 3 天前</div>
          </div>
          <div className="btn-sm">更新</div>
        </div>
        <div className="srow">
          <div className="scol">
            <div className="stit">IP 列表</div>
            <div className="ssub mono">9,662 条 · 3 天前</div>
          </div>
          <div className="btn-sm">更新</div>
        </div>
        <div className="srow">
          <div className="scol">
            <div className="stit">未收录域名</div>
            <div className="ssub">不在列表中的域名如何处理</div>
          </div>
          <div className="seg"><i className="on">走代理</i><i>按 IP 判断</i></div>
        </div>
      </div>

      <div className="shead micro">BEHAVIOR · 行为</div>
      <div className="card" style={{ borderWidth: 1, borderStyle: 'solid', borderColor: 'var(--line)' }}>
        <div className="srow">
          <div className="scol">
            <div className="stit">记录请求明细</div>
            <div className="ssub">每个请求记录直连或代理（量较大）</div>
          </div>
          <div className="sw on"><i></i></div>
        </div>
        <div className="srow">
          <div className="scol">
            <div className="stit">退出时保持运行</div>
            <div className="ssub">关闭 mygost 后 gost 继续工作</div>
          </div>
          <div className="sw on"><i></i></div>
        </div>

      </div>


    </div>
  );
}

function BLogs() {
  return (
    <div className="ab ab-b">
      <div className="subhead">
        <div className="back">←</div>
        <div className="stitle mono" style={{ letterSpacing: '.04em' }}>日志</div>
      </div>

      <div className="tabs">
        <div className="seg" style={{ display: 'flex' }}>
          <i>原始日志</i>
          <i className="on mono" style={{ flex: 1, textAlign: 'center' }}>请求（128）</i>
        </div>
      </div>

      <div className="toolbar">
        <div className="search mono" style={{ fontSize: 12 }}>搜索域名…</div>
        <div className="chip">仅看失败</div>
        <div className="ghost">复制</div>
      </div>

      <div className="loglist" style={{ borderWidth: 1, borderStyle: 'solid', borderColor: 'var(--line)' }}>
        <div className="lrow">
          <div className="lt mono">21:14:32</div>
          <div className="badge b-dir">直连</div>
          <div className="lcol"><div className="host mono">www.baidu.com</div></div>
        </div>
        <div className="lrow">
          <div className="lt mono">21:14:32</div>
          <div className="badge b-dir">直连</div>
          <div className="lcol"><div className="host mono">sp1.baidu.com</div></div>
        </div>
        <div className="lrow">
          <div className="lt mono">21:14:31</div>
          <div className="badge b-prx">代理</div>
          <div className="lcol">
            <div className="host mono">gateway.icloud.com</div>
            <div className="dst micro mono">→ HK · 149.129.41.181:8081</div>
          </div>
        </div>
        <div className="lrow">
          <div className="lt mono">21:14:30</div>
          <div className="badge b-prx">代理</div>
          <div className="lcol">
            <div className="host mono">api.instagram.com</div>
            <div className="dst micro mono">→ HK · 149.129.41.181:8081</div>
          </div>
        </div>
        <div className="lrow">
          <div className="lt mono">21:14:28</div>
          <div className="badge b-err">失败</div>
          <div className="lcol">
            <div className="host mono">registry.npmjs.org</div>
            <div className="dst">连接超时 · 已重试 2 次</div>
          </div>
        </div>
        <div className="lrow">
          <div className="lt mono">21:14:25</div>
          <div className="badge b-dir">直连</div>
          <div className="lcol"><div className="host mono">hector.baidu.com</div></div>
        </div>
        <div className="lrow">
          <div className="lt mono">21:14:24</div>
          <div className="badge b-prx">代理</div>
          <div className="lcol">
            <div className="host mono">chat.openai.com</div>
            <div className="dst micro mono">→ HK · 149.129.41.181:8081</div>
          </div>
        </div>
        <div className="lrow">
          <div className="lt mono">21:14:21</div>
          <div className="badge b-dir">直连</div>
          <div className="lcol"><div className="host mono">dss1.bdstatic.com</div></div>
        </div>
        <div className="lsum mono">共 <b>128</b> 条　直连 <b>86</b> · 代理 <b>39</b> · 失败 <b>3</b></div>
      </div>
    </div>
  );
}
