// 编辑配置两屏（方向 A）：A5 表单·代理 / A6 YAML 高级模式
function AEditorProxy() {
  return (
    <div className="ab ab-a">
      <div className="subhead">
        <div className="back">←</div>
        <div className="stitle">新建配置</div>
      </div>

      <div className="tabs3">
        <div className="seg">
          <i className="on">代理</i>
          <i>端口转发</i>
          <i>YAML</i>
        </div>
      </div>

      <div className="shead">① 本机监听 · 在这台机器上开一个代理端口</div>
      <div className="card">
        <div className="frow">
          <div className="fcol">
            <div className="stit">监听地址</div>
            <div className="ssub">留空则自动选择端口</div>
          </div>
          <div className="fin mono">:8026</div>
        </div>
        <div className="frow">
          <div className="fcol">
            <div className="stit">协议</div>
            <div className="ssub">本机以什么协议对外提供代理</div>
          </div>
          <div className="seg"><i className="on">http</i><i>socks5</i></div>
        </div>
        <div className="frow">
          <div className="fcol">
            <div className="stit">本机认证</div>
            <div className="ssub">可选 · 留空无需认证</div>
          </div>
          <div className="fpair">
            <div className="fin dim">账号</div>
            <div className="fin dim">密码</div>
          </div>
        </div>
      </div>

      <div className="shead">② 出网方式 · 流量从哪里连到目标</div>
      <div className="card">
        <div className="frow">
          <div className="fcol">
            <div className="stit">出网</div>
            <div className="ssub">经上游：先连服务器，由它代连（-F）</div>
          </div>
          <div className="seg"><i>直连</i><i className="on">经上游</i></div>
        </div>
        <div className="upcard">
          <div className="frow">
            <div className="fcol">
              <div className="stit">服务器</div>
              <div className="ssub">协议与地址</div>
            </div>
            <div className="fpair">
              <div className="fin" style={{ width: 66 }}>sshd</div>
              <div className="fin mono" style={{ width: 128 }}>149.129.41.181:8081</div>
            </div>
          </div>
          <div className="frow">
            <div className="fcol">
              <div className="stit">服务器认证</div>
              <div className="ssub">可选 · 留空匿名</div>
            </div>
            <div className="fpair">
              <div className="fin mono" style={{ width: 66 }}>gost</div>
              <div className="fin mono" style={{ width: 128 }}>••••••••</div>
            </div>
          </div>
        </div>
      </div>

      <div className="cmdlab">等效命令</div>
      <div className="cmd mono"><b>gost -L</b>=http://:8026 <b>-F</b> sshd://gost:***@149.129.41.181:8081</div>

      <div className="bbar">
        <div className="btn-cancel">取消</div>
        <div className="btn-save">保存</div>
      </div>
    </div>
  );
}

// YAML 代码块：一行一个 <div>，行号 + 键/值着色
function YLine({ n, children }) {
  return (
    <div style={{ display: 'flex' }}>
      <span className="ln" style={{ width: 24, flex: '0 0 auto' }}>{n}</span>
      <span style={{ flex: 1, minWidth: 0, whiteSpace: 'pre' }}>{children}</span>
    </div>
  );
}

function AEditorYaml() {
  return (
    <div className="ab ab-a">
      <div className="subhead">
        <div className="back">←</div>
        <div className="stitle">编辑配置</div>
      </div>

      <div className="tabs3">
        <div className="seg">
          <i>代理</i>
          <i>端口转发</i>
          <i className="on">YAML</i>
        </div>
      </div>

      <div className="shead">高级模式 · 完整 gost 配置，保存后覆盖表单字段</div>
      <div className="code">
        <YLine n="1"><span className="k">services</span>:</YLine>
        <YLine n="2">{"  - "}<span className="k">name</span>: <span className="v">mygost</span></YLine>
        <YLine n="3">{"    "}<span className="k">addr</span>: <span className="v">":8026"</span></YLine>
        <YLine n="4">{"    "}<span className="k">handler</span>:</YLine>
        <YLine n="5">{"      "}<span className="k">type</span>: <span className="v">http</span></YLine>
        <YLine n="6">{"      "}<span className="k">chain</span>: <span className="v">chain-0</span></YLine>
        <YLine n="7">{"    "}<span className="k">listener</span>:</YLine>
        <YLine n="8">{"      "}<span className="k">type</span>: <span className="v">tcp</span></YLine>
        <YLine n="9"><span className="k">chains</span>:</YLine>
        <YLine n="10">{"  - "}<span className="k">name</span>: <span className="v">chain-0</span></YLine>
        <YLine n="11">{"    "}<span className="k">hops</span>:</YLine>
        <YLine n="12">{"      - "}<span className="k">name</span>: <span className="v">hop-0</span></YLine>
        <YLine n="13">{"        "}<span className="k">bypasses</span>: <span className="c"># 分流（mygost 自动维护）</span></YLine>
        <YLine n="14">{"          - "}<span className="v">split-private</span></YLine>
        <YLine n="15">{"          - "}<span className="v">split-geo</span></YLine>
        <YLine n="16">{"        "}<span className="k">nodes</span>:</YLine>
        <YLine n="17">{"          - "}<span className="k">addr</span>: <span className="v">149.129.41.181:8081</span></YLine>
        <YLine n="18">{"            "}<span className="k">connector</span>: {"{ "}<span className="k">type</span>: <span className="v">sshd</span>{" }"}</YLine>
        <YLine n="19">{"            "}<span className="k">dialer</span>: {"{ "}<span className="k">type</span>: <span className="v">sshd</span>, <span className="k">auth</span>: <span className="c">…</span>{" }"}</YLine>
      </div>

      <div className="vok"><span className="dot dot-grn"></span>YAML 校验通过 · 19 行</div>

      <div className="bbar">
        <div className="btn-cancel">取消</div>
        <div className="btn-save">校验并保存</div>
      </div>
    </div>
  );
}

// A7 · 编辑 · 端口转发：监听 + 目标列表（多个即负载均衡）+ 名称
function AEditorForward() {
  return (
    <div className="ab ab-a">
      <div className="subhead">
        <div className="back">←</div>
        <div className="stitle">新建配置</div>
      </div>

      <div className="tabs3">
        <div className="seg">
          <i>代理</i>
          <i className="on">端口转发</i>
          <i>YAML</i>
        </div>
      </div>

      <div className="shead">① 本机监听 · 在这台机器上开一个端口</div>
      <div className="card">
        <div className="frow">
          <div className="fcol">
            <div className="stit">监听地址</div>
            <div className="ssub">留空则自动选择端口</div>
          </div>
          <div className="fin mono">:2222</div>
        </div>
        <div className="frow">
          <div className="fcol">
            <div className="stit">协议</div>
            <div className="ssub">tcp 适用于 SSH、网页等服务</div>
          </div>
          <div className="seg"><i className="on">tcp</i><i>udp</i></div>
        </div>
      </div>

      <div className="shead">② 转发到 · 连接本机端口时，实际连到这些地址</div>
      <div className="card">
        <div className="ta">
          <div className="trow">
            <div className="taddr mono">192.168.1.10:22</div>
            <div className="tdel">✕</div>
          </div>
          <div className="trow">
            <div className="taddr mono">192.168.1.11:22</div>
            <div className="tdel">✕</div>
          </div>
          <div className="taadd">＋ 添加目标</div>
        </div>
      </div>
      <div className="vok"><span className="dot dot-acc"></span>多个目标自动负载均衡 · 按顺序依次尝试</div>

      <div className="shead">③ 名称 · 只用于列表显示</div>
      <div className="card">
        <div className="frow">
          <div className="fin mono" style={{ width: '100%' }}>家里 NAS</div>
        </div>
      </div>

      <div className="cmdlab">等效命令</div>
      <div className="cmd mono"><b>gost -L</b>=tcp://:2222/192.168.1.10:22,192.168.1.11:22</div>

      <div className="bbar">
        <div className="btn-cancel">取消</div>
        <div className="btn-save">保存</div>
      </div>
    </div>
  );
}
