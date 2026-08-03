package desk

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// 调色板 — 浅色液态玻璃风格
var (
	colorBg          = color.RGBA{R: 240, G: 244, B: 248, A: 255}
	colorCardBg      = color.RGBA{R: 255, G: 255, B: 255, A: 200}
	colorCardHover   = color.RGBA{R: 255, G: 255, B: 255, A: 235}
	colorAccent      = color.RGBA{R: 0, G: 122, B: 255, A: 255}
	colorAccentDim   = color.RGBA{R: 0, G: 122, B: 255, A: 40}
	colorTextPrimary = color.RGBA{R: 28, G: 32, B: 38, A: 255}
	colorTextDim     = color.RGBA{R: 142, G: 150, B: 162, A: 255}
	colorGreen       = color.RGBA{R: 52, G: 199, B: 89, A: 255}
	colorRed         = color.RGBA{R: 255, G: 69, B: 58, A: 255}
	colorSeparator   = color.RGBA{R: 220, G: 226, B: 234, A: 255}
	colorSkeleton    = color.RGBA{R: 224, G: 230, B: 238, A: 255}
)

// glassTheme 自定义浅色液态玻璃主题
type glassTheme struct {
	fyne.Theme
}

func newGlassTheme() fyne.Theme {
	return &glassTheme{theme.DefaultTheme()}
}

func (g *glassTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return colorBg
	case theme.ColorNameButton:
		return colorCardBg
	case theme.ColorNameInputBackground:
		return colorCardBg
	case theme.ColorNameForeground:
		return colorTextPrimary
	case theme.ColorNameDisabled:
		return colorTextDim
	case theme.ColorNameSeparator:
		return colorSeparator
	case theme.ColorNameSelection:
		return colorAccentDim
	case theme.ColorNamePrimary:
		return colorAccent
	case theme.ColorNameError:
		return colorRed
	case theme.ColorNameSuccess:
		return colorGreen
	case theme.ColorNameHover:
		return colorCardHover
	case theme.ColorNameHeaderBackground:
		return colorCardBg
	case theme.ColorNameMenuBackground:
		return colorCardBg
	case theme.ColorNameOverlayBackground:
		return colorCardBg
	default:
		return g.Theme.Color(name, theme.VariantLight)
	}
}

func (g *glassTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 10
	case theme.SizeNameInnerPadding:
		return 14
	case theme.SizeNameText:
		return 14
	case theme.SizeNameSeparatorThickness:
		return 1
	default:
		return g.Theme.Size(name)
	}
}

func (g *glassTheme) Radius() float32 {
	return 12
}

// card 创建圆角卡片容器，带内边距
func card(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(colorCardBg)
	bg.CornerRadius = 12
	return container.NewStack(
		bg,
		container.NewPadded(content),
	)
}

// cardWithPadding 创建带自定义内边距的卡片
func cardWithPadding(content fyne.CanvasObject, pad float32) fyne.CanvasObject {
	bg := canvas.NewRectangle(colorCardBg)
	bg.CornerRadius = 12
	return container.NewStack(
		bg,
		paddedWithInsets(pad, pad, pad, pad, content),
	)
}

// sectionCard 创建带标题的分区卡片
func sectionCard(title string, content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(colorCardBg)
	bg.CornerRadius = 12
	titleLabel := widget.NewLabel(title)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	return container.NewStack(
		bg,
		container.NewBorder(
			paddedWithInsets(16, 16, 14, 14, titleLabel),
			nil, nil, nil,
			paddedWithInsets(0, 16, 16, 16, content),
		),
	)
}

// skeletonLabel 骨架屏占位
func skeletonLabel() fyne.CanvasObject {
	bg := canvas.NewRectangle(colorSkeleton)
	bg.CornerRadius = 4
	return container.NewStack(
		bg,
		widget.NewLabel(""),
	)
}

// skeletonRow 创建一行骨架屏（n 个占位块）
func skeletonRow(n int) fyne.CanvasObject {
	items := make([]fyne.CanvasObject, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, skeletonLabel())
	}
	return container.NewGridWithColumns(n, items...)
}

// skeletonBlock 创建多行骨架屏
func skeletonBlock(rows, cols int) fyne.CanvasObject {
	rowObjs := make([]fyne.CanvasObject, 0, rows)
	for i := 0; i < rows; i++ {
		rowObjs = append(rowObjs, skeletonRow(cols))
	}
	return container.NewVBox(rowObjs...)
}

// emptyState 空状态：图标 + 引导文字
func emptyState(icon, text string) fyne.CanvasObject {
	iconText := canvas.NewText(icon, colorTextDim)
	iconText.TextSize = 32
	iconText.TextStyle = fyne.TextStyle{Bold: true}
	textObj := canvas.NewText(text, colorTextDim)
	textObj.TextSize = 14

	return container.NewCenter(
		container.NewVBox(
			container.NewCenter(iconText),
			spacer(8),
			container.NewCenter(textObj),
		),
	)
}

// spacer 留白
func spacer(h float32) fyne.CanvasObject {
	return container.NewBorder(nil, nil, nil, nil, canvas.NewRectangle(color.Transparent))
}

// hSpacer 水平留白
func hSpacer(w float32) fyne.CanvasObject {
	return container.NewBorder(nil, nil, nil, nil, canvas.NewRectangle(color.Transparent))
}

// paddedWithInsets creates a container with custom padding on all sides.
func paddedWithInsets(top, bottom, left, right float32, content fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(
		canvas.NewRectangle(color.Transparent),
		canvas.NewRectangle(color.Transparent),
		canvas.NewRectangle(color.Transparent),
		canvas.NewRectangle(color.Transparent),
		content,
	)
}

// coloredLabel creates a canvas.Text with a specific color.
func coloredLabel(text string, c color.Color) *canvas.Text {
	t := canvas.NewText(text, c)
	t.TextSize = 14
	return t
}

// statCard 数据卡片：标题 + 数值
func statCard(label, value string) fyne.CanvasObject {
	bg := canvas.NewRectangle(colorCardBg)
	bg.CornerRadius = 10
	l := coloredLabel(label, colorTextDim)
	v := canvas.NewText(value, colorTextPrimary)
	v.TextStyle = fyne.TextStyle{Bold: true}
	v.TextSize = 18
	return container.NewStack(
		bg,
		paddedWithInsets(16, 16, 16, 16,
			container.NewVBox(l, spacer(4), v),
		),
	)
}

// hoverCard 带悬浮变色效果的卡片
type hoverCard struct {
	widget.BaseWidget
	bg     *canvas.Rectangle
	content fyne.CanvasObject
}

func newHoverCard(content fyne.CanvasObject) *hoverCard {
	hc := &hoverCard{
		bg:      canvas.NewRectangle(colorCardBg),
		content: content,
	}
	hc.bg.CornerRadius = 12
	hc.ExtendBaseWidget(hc)
	return hc
}

func (h *hoverCard) CreateRenderer() fyne.WidgetRenderer {
	return &hoverCardRenderer{hc: h}
}

type hoverCardRenderer struct {
	hc *hoverCard
}

func (r *hoverCardRenderer) Layout(size fyne.Size) {
	r.hc.bg.Resize(size)
	r.hc.content.Resize(fyne.NewSize(size.Width-28, size.Height-28))
	r.hc.content.Move(fyne.NewPos(14, 14))
}

func (r *hoverCardRenderer) MinSize() fyne.Size {
	return r.hc.content.MinSize().AddWidthHeight(28, 28)
}

func (r *hoverCardRenderer) Refresh() {
	r.hc.bg.Refresh()
	r.hc.content.Refresh()
}

func (r *hoverCardRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.hc.bg, r.hc.content}
}

func (r *hoverCardRenderer) Destroy() {}

func (r *hoverCardRenderer) MouseIn(*fyne.PointEvent) {
	r.hc.bg.FillColor = colorCardHover
	r.hc.bg.Refresh()
}

func (r *hoverCardRenderer) MouseOut() {
	r.hc.bg.FillColor = colorCardBg
	r.hc.bg.Refresh()
}

func (r *hoverCardRenderer) MouseDown(*fyne.PointEvent) {}
func (r *hoverCardRenderer) MouseUp(*fyne.PointEvent)   {}
func (r *hoverCardRenderer) Dragged(*fyne.DragEvent)    {}
func (r *hoverCardRenderer) DragEnd()                   {}
