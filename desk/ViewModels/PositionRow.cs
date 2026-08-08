using ArbDesk.Dashboard;

namespace ArbDesk.ViewModels;

public sealed class PositionRow
{
    public string Broker { get; }
    public long Ticket { get; }
    public string Symbol { get; }
    public string Side { get; }
    public string Lots { get; }
    public string OpenPrice { get; }
    public string CurrentPrice { get; }
    public string FloatingPnl { get; }
    public string SwapAccrued { get; }
    public string Commission { get; }
    public string OpenTime { get; }
    public string Comment { get; }

    public PositionRow(string broker, PositionWatchReply.Types.Position pos)
    {
        Broker = broker;
        Ticket = pos.Ticket;
        Symbol = pos.Symbol;
        Side = pos.Side;
        Lots = $"{pos.Lots:F2}";
        OpenPrice = $"{pos.OpenPrice:F5}";
        CurrentPrice = $"{pos.CurrentPrice:F5}";
        FloatingPnl = $"{pos.FloatingPnl:F2}";
        SwapAccrued = $"{pos.SwapAccrued:F2}";
        Commission = $"{pos.Commission:F2}";
        OpenTime = DateTimeOffset
            .FromUnixTimeMilliseconds(pos.OpenTimeUnixMs)
            .ToString("MM-dd HH:mm");
        Comment = pos.Comment;
    }
}
