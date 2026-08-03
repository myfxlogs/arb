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
}

// NewHistoryTab creates a history tab.
func NewHistoryTab(client dashpb.DashboardServiceClient) fyne.CanvasObject {
	h := &HistoryTab{client: client}

	signalList := widget.NewList(
		func() int { return len(h.signals) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if i < len(h.signals) {
				obj.(*widget.Label).SetText(fmt.Sprintf("%s %s executed=%v",
					h.signals[i].Id, h.signals[i].Strategy, h.signals[i].Executed))
			}
		},
	)

	orderList := widget.NewList(
		func() int { return len(h.orders) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if i < len(h.orders) {
				obj.(*widget.Label).SetText(fmt.Sprintf("%s %s %s %.2f",
					h.orders[i].ClientId, h.orders[i].Broker, h.orders[i].Symbol, h.orders[i].Volume))
			}
		},
	)

	refreshBtn := widget.NewButton("Refresh", func() {
		h.refresh()
		signalList.Refresh()
		orderList.Refresh()
	})

	return container.NewBorder(
		container.NewHBox(widget.NewLabel("History"), refreshBtn),
		nil, nil, nil,
		container.NewAppTabs(
			container.NewTabItem("Signals", signalList),
			container.NewTabItem("Orders", orderList),
		),
	)
}

func (h *HistoryTab) refresh() {
	now := time.Now()
	from := now.AddDate(0, -1, 0) // last month

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
}
