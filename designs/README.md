# 设计稿存档

`ui-redesign/` 是 0.2.0 界面重构时的设计稿（HTML 原型，与实现对照用）。

- `index.html` — 交互画布，两个方向各 4~7 屏并排对比
- `sheets.html` — 静态总览，一页看完全部屏幕
- `ui.css` — 设计 token 与组件样式（浅色 / 深色两套色彩变量）
- `vendor/` — React + Babel 的本地副本（离线可开，不依赖 CDN）

本地查看：

```bash
python3 -m http.server 4311 --directory designs
# 打开 http://localhost:4311/ui-redesign/index.html
```

最终采用的是**方向 A「澄澈」**（浅色灰底白卡 + 品牌蓝），
深色模式借用方向 B「曜石」的配色。代码里的实现见 `theme.go`。
