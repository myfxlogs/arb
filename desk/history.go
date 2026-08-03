package desk

import (
	"context"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	dashpb "arb/proto/gen/dashboard"
)

// HistoryTab displays signal and order history.
type HistoryTab struct {
	client  dashpb.DashboardServiceClient
	signals []*dashpb.SignalHistoryReply_SignalItem
	orders  []*dashpb.OrderHistoryReply_OrderItem
	sigList  *widget.List
	ordList  *widget.List
	loaded   bool
}

// NewHistoryTab creates a history tab.
func NewHistoryTab(client dashpb.DashboardServiceClient) fyne.CanvasObject {
	h := &HistoryTab{client: client}

	h.sigList = widget.NewList(
		func() int { return len(h.signals) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if i < len(h.signals) {
				obj.(*widget.Label).SetText(fmt.Sprintf("%s  %s  已执行=%v",
					h.signals[i].Id, h.signals[i].Strategy, h.signals[i].Executed))
			}
		},
	)

	h.ordList = widget.NewList(
		func() int { return len(h.orders) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if i < len(h.orders) {
				obj.(*widget.Label).SetText(fmt.Sprintf("%s  %s  %s  %.2f手",
					h.orders[i].ClientId, h.orders[i].Broker, h.orders[i].Symbol, h.orders[i].Volume))
			}
		},
	)

	refreshBtn := widget.NewButton("刷新", func() {
		h.refresh()
	})

	signalTab := container.NewStack(h.sigList)
	orderTab := container.NewStack(h.ordList)

	historyTabs := container.NewAppTabs(
		container.NewTabItem("信号", signalTab),
		container.NewTabItem("订单", orderTab),
	)

	header := container.NewHBox(widget.NewLabel("历史记录"), spacer(0), refreshBtn)

	return container.NewBorder(
		paddedWithInsets(16, 16, 16, 16, header),
		nil, nil, nil,
		paddedWithInsets(0, 16, 16, 16, historyTabs),
	)
}

func (h *HistoryTab) refresh() {
	now := time.Now()
	from := now.AddDate(0, -1, 0)

	sigReply, err := h.client.GetSignalHistory(context.Background(), &dashpb.SignalHistoryRequest{
		FromUnixMs: from.UnixMilli(),
		ToUnixMs:   now.UnixMilli(),
		Limit:      100,
	})
	if err == nil {
		h.signals = sigReply.Items
	}

	orderReply, err := h.client.GetOrderHistory(context.Background(), &dashpb.OrderHistoryRequest{
		FromUnixMs: from.UnixMilli(),
		ToUnixMs:   now.UnixMilli(),
		Limit:      100,
	})
	if err == nil {
		h.orders = orderReply.Items
	}
	h.loaded = true
	h.sigList.Refresh()
	h.ordList.Refresh()
}
