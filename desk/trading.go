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
	brokerEntry.SetPlaceHolder("Broker name")
	symbolEntry := widget.NewEntry()
	symbolEntry.SetPlaceHolder("Symbol")
	sideSelect := widget.NewSelect([]string{"Buy", "Sell"}, nil)
	sideSelect.SetSelected("Buy")
	lotsEntry := widget.NewEntry()
	lotsEntry.SetPlaceHolder("0.1")
	priceEntry := widget.NewEntry()
	priceEntry.SetPlaceHolder("0 (market)")
	slippageEntry := widget.NewEntry()
	slippageEntry.SetPlaceHolder("0")

	resultLabel := widget.NewLabel("")

	submitBtn := widget.NewButton("Submit Order", func() {
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
			resultLabel.SetText(fmt.Sprintf("Error: %v", err))
			return
		}
		resultLabel.SetText(fmt.Sprintf("Status: %s Ticket: %d", reply.Status, reply.Ticket))
	})

	form := container.NewVBox(
		widget.NewLabel("Manual Order"),
		brokerEntry,
		symbolEntry,
		sideSelect,
		lotsEntry,
		priceEntry,
		slippageEntry,
		submitBtn,
		resultLabel,
	)
	return form
}
