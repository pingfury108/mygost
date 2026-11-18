// 说明板：设计目标、两方向、落地约束
function IntroBoard() {
  return (
    <div className="readme">
      <h1>mygost · UI 重设计</h1>
      <div className="ver">430 × 660 · 与现有窗口一致 · 界面语言：中文</div>

      <h2>这次要解决的三个问题</h2>
      <li>主界面状态感弱：按钮孤立、下半屏空白，连接后"发生了什么"不可见</li>
      <li>速率与流量存在感不足：只有一行小字，错过了实时反馈的价值</li>
      <li>分流状态藏在设置里：插件是否在跑、规则是否就绪，主界面看不到</li>

      <h2>两个方向</h2>
      <div className="dcard">
        <b><span className="sw2" style={{ background: '#2563EB' }}></span>A · 澄澈</b>
        <p>浅色精修。保留现有蓝白基调与 iOS 质感，重排主界面：状态徽章 + 速率卡成为视觉重心，配置列表更聚焦。改动最小，与现有代码气质一致。</p>
      </div>
      <div className="dcard">
        <b><span className="sw2" style={{ background: '#0C0E11' }}></span>B · 曜石</b>
        <p>深色工具风。近黑底 + 青色强调，等宽数字贯穿速率/流量/地址，统计瓦片带速率条。更像网络工具的专业感，适合常驻菜单栏场景。</p>
      </div>

      <h2>落地约束（不画做不到的稿）</h2>
      <li>全部纯色 + 圆角 + 文字层级，MyGo 原生控件可实现</li>
      <li>大圆按钮光环 = 现有 successHalo 方案，仅放大外环</li>
      <li>开关 / 分段控件 / 徽章 = 现有控件样式微调，无新依赖</li>
      <li>速率条（方向 B）= 简单圆角矩形填充，可画</li>

      <div className="hint">
        画布可拖拽平移、滚轮缩放；每张卡片右上角 ⤢ 可全屏查看。
        每个方向 4 屏：主界面·已连接 / 主界面·未连接 / 设置·分流 / 日志·请求。
      </div>
    </div>
  );
}
