// 静态总览页：两方向四屏并排，便于整体对比与截图
function App() {
  const cells = (ids) => ids.map((c) => (
    <div className="cell" key={c.id}>
      <div className="frame">{c.el}</div>
      <div className="cap">{c.cap}</div>
    </div>
  ));

  const rowA = cells([
    { id: 'a1', el: <AMainOn></AMainOn>, cap: 'A1 · 主界面 · 已连接' },
    { id: 'a2', el: <AMainOff></AMainOff>, cap: 'A2 · 主界面 · 未连接' },
    { id: 'a3', el: <ASettings></ASettings>, cap: 'A3 · 设置 · 分流' },
    { id: 'a4', el: <ALogs></ALogs>, cap: 'A4 · 日志 · 请求' },
    { id: 'a5', el: <AEditorProxy></AEditorProxy>, cap: 'A5 · 编辑 · 代理' },
    { id: 'a6', el: <AEditorYaml></AEditorYaml>, cap: 'A6 · 编辑 · YAML' },
    { id: 'a7', el: <AEditorForward></AEditorForward>, cap: 'A7 · 编辑 · 端口转发' },
  ]);

  const rowB = cells([
    { id: 'b1', el: <BMainOn></BMainOn>, cap: 'B1 · 主界面 · 已连接' },
    { id: 'b2', el: <BMainOff></BMainOff>, cap: 'B2 · 主界面 · 未连接' },
    { id: 'b3', el: <BSettings></BSettings>, cap: 'B3 · 设置 · 分流' },
    { id: 'b4', el: <BLogs></BLogs>, cap: 'B4 · 日志 · 请求' },
  ]);

  return (
    <div className="sheet">
      <div className="rowhead">方向 A · 澄澈 <span>浅色精修 · 保留现有蓝白基调 · 改动最小</span></div>
      <div className="row">{rowA}</div>
      <div className="rowhead">方向 B · 曜石 <span>深色工具风 · 近黑底 + 青色强调 · 等宽数字</span></div>
      <div className="row">{rowB}</div>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<App></App>);
