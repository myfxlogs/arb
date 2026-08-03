package desk

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	dashpb "arb/proto/gen/dashboard"
)

// TradingTab provides manual order submission and position close.
type TradingTab struct {
	client dashpb.DashboardServiceClient
}

// NewTradingTab creates a trading tab.
func NewTradingTab(client dashpb.DashboardServiceClient) fyne.CanvasObject {
	t := &TradingTab{client: client}

	brokerEntry := widget.NewEntry()
	brokerEntry.SetPlaceHolder("经纪商名称")
	symbolEntry := widget.NewEntry()
	symbolEntry.SetPlaceHolder("品种")
	sideSelect := widget.NewSelect([]string{"买入", "卖出"}, nil)
	sideSelect.SetSelected("买入")
	lotsEntry := widget.NewEntry()
	lotsEntry.SetPlaceHolder("0.1")
	priceEntry := widget.NewEntry()
	priceEntry.SetPlaceHolder("0 (市价)")
	slippageEntry := widget.NewEntry()
	slippageEntry.SetPlaceHolder("0")

	resultLabel := widget.NewLabel("")

	submitBtn := widget.NewButton("提交订单", func() {
		req := &dashpb.ManualOrderRequest{
			BrokerName: brokerEntry.Text,
			Symbol:     symbolEntry.Text,
			Side:       sideSelect.Selected,
		}
		req.Lots = parseFloat(lotsEntry.Text)
		req.Price = parseFloat(priceEntry.Text)
		req.Slippage = int32(parseInt(slippageEntry.Text))
		reply, err := t.client.SubmitOrder(context.Background(), req)
		if err != nil {
			resultLabel.SetText(fmt.Sprintf("错误: %v", err))
			return
		}
		resultLabel.SetText(fmt.Sprintf("状态: %s  订单号: %d", reply.Status, reply.Ticket))
	})

	formContent := container.NewVBox(
		widget.NewLabel("经纪商"), brokerEntry,
		spacer(8),
		widget.NewLabel("品种"), symbolEntry,
		spacer(8),
		widget.NewLabel("方向"), sideSelect,
		spacer(8),
		container.NewGridWithColumns(2,
			container.NewVBox(widget.NewLabel("手数"), lotsEntry),
			container.NewVBox(widget.NewLabel("价格"), priceEntry),
		),
		spacer(8),
		widget.NewLabel("滑点"), slippageEntry,
		spacer(16),
		submitBtn,
		spacer(12),
		resultLabel,
	)

	return paddedWithInsets(40, 40, 40, 40,
		sectionCard("手动下单", formContent),
	)
}
