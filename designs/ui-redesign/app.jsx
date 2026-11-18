const { DesignCanvas, DCSection, DCArtboard } = window;

function App() {
  return (
    <DesignCanvas>
      <DCSection id="intro" title="说明" subtitle="目标 · 两方向 · 落地约束">
        <DCArtboard id="readme" label="README" width={430} height={760}>
          <IntroBoard></IntroBoard>
        </DCArtboard>
      </DCSection>

      <DCSection id="dir-a" title="方向 A · 澄澈" subtitle="浅色精修 · 改动最小">
        <DCArtboard id="a1" label="A1 · 主界面 已连接" width={430} height={660}>
          <AMainOn></AMainOn>
        </DCArtboard>
        <DCArtboard id="a2" label="A2 · 主界面 未连接" width={430} height={660}>
          <AMainOff></AMainOff>
        </DCArtboard>
        <DCArtboard id="a3" label="A3 · 设置 分流" width={430} height={660}>
          <ASettings></ASettings>
        </DCArtboard>
        <DCArtboard id="a4" label="A4 · 日志 请求" width={430} height={660}>
          <ALogs></ALogs>
        </DCArtboard>
        <DCArtboard id="a5" label="A5 · 编辑 代理" width={430} height={660}>
          <AEditorProxy></AEditorProxy>
        </DCArtboard>
        <DCArtboard id="a6" label="A6 · 编辑 YAML" width={430} height={660}>
          <AEditorYaml></AEditorYaml>
        </DCArtboard>
        <DCArtboard id="a7" label="A7 · 编辑 端口转发" width={430} height={660}>
          <AEditorForward></AEditorForward>
        </DCArtboard>
      </DCSection>

      <DCSection id="dir-b" title="方向 B · 曜石" subtitle="深色工具风 · 数字感">
        <DCArtboard id="b1" label="B1 · 主界面 已连接" width={430} height={660}>
          <BMainOn></BMainOn>
        </DCArtboard>
        <DCArtboard id="b2" label="B2 · 主界面 未连接" width={430} height={660}>
          <BMainOff></BMainOff>
        </DCArtboard>
        <DCArtboard id="b3" label="B3 · 设置 分流" width={430} height={660}>
          <BSettings></BSettings>
        </DCArtboard>
        <DCArtboard id="b4" label="B4 · 日志 请求" width={430} height={660}>
          <BLogs></BLogs>
        </DCArtboard>
      </DCSection>
    </DesignCanvas>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<App></App>);
