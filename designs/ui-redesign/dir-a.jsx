// 方向 A「澄澈」：浅色精修，四屏
function AMainOn() {
  return (
    <div className="ab ab-a">
      <div className="top">
        <div className="brand">mygost</div>
        <div className="pill"><span className="dot dot-grn"></span>已连接 <span className="mono">02:41:05</span></div>
      </div>

      <div className="center">
        <div className="big big-stop">断开</div>
        <div className="cur">
          <div className="cur-name">hk</div>
          <div className="cur-sub mono">:8026 · http · 经上游 149.129.41.181:8081</div>
        </div>
      </div>

      <div className="rate">
        <div className="rate-grid">
          <div className="rcell">
            <div className="rlab">上传</div>
            <div className="rval rval-up mono">1.2 MB/s</div>
          </div>
          <div className="rcell">
            <div className="rlab">下载</div>
            <div className="rval rval-dn mono">8.4 MB/s</div>
          </div>
        </div>
        <div className="sess mono">本次会话　↑ 142 MB　·　↓ 1.2 GB</div>
      </div>

      <div className="sec">
        <div className="sec-t">配置</div>
        <div className="sec-a">＋ 添加</div>
      </div>
      <div className="list">
        <div className="row sel">
          <div className="chk">✓</div>
          <div className="rgrow">
            <div className="rname">hk　<span className="tag">代理</span></div>
            <div className="rsub mono">:8026 · 经上游 149.129.41.181:8081</div>
          </div>
          <div className="more">⋯</div>
        </div>
        <div className="row">
          <div className="rgrow">
            <div className="rname">nas　<span className="tag">端口转发</span></div>
            <div className="rsub mono">:2222 → 192.168.1.10:22</div>
          </div>
          <div className="more">⋯</div>
        </div>
      </div>

      <div className="foot"><span className="dot dot-grn"></span>分流运行中 · 规则已就绪</div>
    </div>
  );
}

function AMainOff() {
  return (
    <div className="ab ab-a">
      <div className="top">
        <div className="brand">mygost</div>
        <div className="pill"><span className="dot dot-mut"></span>未连接</div>
      </div>

      <div className="center">
        <div className="big big-go">连接</div>
        <div className="cur">
          <div className="cur-name">hk</div>
          <div className="cur-sub">点击连接当前选中的配置</div>
        </div>
      </div>

      <div className="sec">
        <div className="sec-t">分流</div>
        <div className="sec-a">设置</div>
      </div>
      <div className="list">
        <div className="row">
          <div className="rgrow">
            <div className="stit">国内直连</div>
            <div className="ssub">已开启 · 110,177 个域名 · 9,662 条 IP</div>
          </div>
          <div className="sw on" style={{ width: 38, height: 22 }}><i style={{ width: 18, height: 18 }}></i></div>
        </div>
      </div>

      <div className="sec">
        <div className="sec-t">配置</div>
        <div className="sec-a">＋ 添加</div>
      </div>
      <div className="list">
        <div className="row sel">
          <div className="chk">✓</div>
          <div className="rgrow">
            <div className="rname">hk　<span className="tag">代理</span></div>
            <div className="rsub mono">:8026 · 经上游 149.129.41.181:8081</div>
          </div>
          <div className="more">⋯</div>
        </div>
        <div className="row">
          <div className="rgrow">
            <div className="rname">nas　<span className="tag">端口转发</span></div>
            <div className="rsub mono">:2222 → 192.168.1.10:22</div>
          </div>
          <div className="more">⋯</div>
        </div>
      </div>

      <div className="foot">连接后可查看实时速率与流量</div>
    </div>
  );
}

function ASettings() {
  return (
    <div className="ab ab-a">
      <div className="subhead">
        <div className="back">←</div>
        <div className="stitle">设置</div>
      </div>

      <div className="shead">分流</div>
      <div className="card">
        <div className="srow">
          <div className="scol">
            <div className="stit">国内直连</div>
            <div className="ssub">国内域名与 IP 直连，其余走隧道</div>
          </div>
          <div className="sw on"><i></i></div>
        </div>
        <div className="srow">
          <div className="scol">
            <div className="stit"><span className="dot dot-grn" style={{ display: 'inline-block', marginRight: 6 }}></span>插件运行中</div>
            <div className="ssub mono">127.0.0.1:8100 · 载入 9,662 IP / 110,177 域名</div>
          </div>
        </div>
        <div className="srow">
          <div className="scol">
            <div className="stit">域名列表</div>
            <div className="ssub">110,177 条 · 更新于 3 天前</div>
          </div>
          <div className="btn-sm">更新</div>
        </div>
        <div className="srow">
          <div className="scol">
            <div className="stit">IP 列表</div>
            <div className="ssub">9,662 条 · 更新于 3 天前</div>
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

      <div className="shead">行为</div>
      <div className="card">
        <div className="srow">
          <div className="scol">
            <div className="stit">记录请求明细</div>
            <div className="ssub">每个请求记录直连或代理（debug 日志，量较大）</div>
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

function ALogs() {
  return (
    <div className="ab ab-a">
      <div className="subhead">
        <div className="back">←</div>
        <div className="stitle">日志</div>
      </div>

      <div className="tabs">
        <div className="seg" style={{ display: 'flex' }}>
          <i>原始日志</i>
          <i className="on" style={{ flex: 1, textAlign: 'center' }}>请求（128）</i>
        </div>
      </div>

      <div className="toolbar">
        <div className="search">搜索域名…</div>
        <div className="chip">仅看失败</div>
        <div className="ghost">复制</div>
      </div>

      <div className="loglist">
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
            <div className="dst mono">→ hk · 149.129.41.181:8081</div>
          </div>
        </div>
        <div className="lrow">
          <div className="lt mono">21:14:30</div>
          <div className="badge b-prx">代理</div>
          <div className="lcol">
            <div className="host mono">api.instagram.com</div>
            <div className="dst mono">→ hk · 149.129.41.181:8081</div>
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
            <div className="dst mono">→ hk · 149.129.41.181:8081</div>
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
