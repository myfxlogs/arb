package desk

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	"image/color"

	dashpb "arb/proto/gen/dashboard"
)

// MatrixTab displays the spread matrix in a table.
type MatrixTab struct {
	widget.Table
	client dashpb.DashboardServiceClient
	data   *dashpb.SpreadMatrixReply
	mu     sync.RWMutex
}

// NewMatrixTab creates a spread matrix tab.
func NewMatrixTab(client dashpb.DashboardServiceClient) *MatrixTab {
	m := &MatrixTab{client: client}
	m.Table = *widget.NewTable(
		func() (int, int) { return m.rows(), m.cols() },
		func() fyne.CanvasObject { return canvas.NewText("", color.White) },
		m.updateCell,
	)
	go m.streamLoop()
	return m
}

func (m *MatrixTab) rows() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.data == nil || len(m.data.Rows) == 0 {
		return 1
	}
	return len(m.data.Rows) + 1 // header row
}

func (m *MatrixTab) cols() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.data == nil || len(m.data.Rows) == 0 {
		return 1
	}
	return int(m.data.TotalSymbols) + 1 // broker name column
}

func (m *MatrixTab) updateCell(id widget.TableCellID, cell fyne.CanvasObject) {
	txt := cell.(*canvas.Text)
	m.mu.RLock()
	defer m.mu.RUnlock()

	if id.Row == 0 {
		if id.Col == 0 {
			txt.Text = "Broker"
		} else if m.data != nil && int(id.Col-1) < len(m.data.Rows[0].Cells) {
			txt.Text = m.data.Rows[0].Cells[id.Col-1].Symbol
		} else {
			txt.Text = ""
		}
		return
	}

	rowIdx := id.Row - 1
	if m.data == nil || rowIdx >= len(m.data.Rows) {
		txt.Text = ""
		return
	}
	row := m.data.Rows[rowIdx]
	if id.Col == 0 {
		txt.Text = row.BrokerName
		return
	}
	if int(id.Col-1) >= len(row.Cells) {
		txt.Text = ""
		return
	}
	c := row.Cells[id.Col-1]
	if c.IsArbitrageable {
		txt.Color = color.RGBA{G: 255, A: 255}
	} else {
		txt.Color = color.White
	}
	txt.Text = fmt.Sprintf("%.1f", c.SpreadToBestAskBps)
}

func (m *MatrixTab) streamLoop() {
	ctx := context.Background()
	stream, err := m.client.SpreadMatrix(ctx, &dashpb.SpreadMatrixRequest{
		RefreshIntervalMs: 200,
	})
	if err != nil {
		slog.Error("matrix stream", "error", err)
		return
	}
	for {
		reply, err := stream.Recv()
		if err != nil {
			slog.Warn("matrix recv", "error", err)
			time.Sleep(time.Second)
			continue
		}
		m.mu.Lock()
		m.data = reply
		m.mu.Unlock()
		m.Table.Refresh()
	}
}
