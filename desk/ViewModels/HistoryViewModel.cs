using System;
using System.Collections.ObjectModel;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using ArbDesk.Dashboard;
using ArbDesk.Services;

namespace ArbDesk.ViewModels;

public partial class HistoryViewModel : ObservableObject
{
    DashboardClient? _client;

    public ObservableCollection<SignalHistoryRow> Signals { get; } = new();

    [ObservableProperty] string _strategyFilter = "";
    [ObservableProperty] string _limitText = "100";
    [ObservableProperty] string _statusMessage = "";

    public void Initialize(DashboardClient client) => _client = client;

    [RelayCommand]
    async Task Load()
    {
        if (_client == null) return;
        if (!int.TryParse(LimitText, out var limit) || limit <= 0)
            limit = 100;

        var now = DateTimeOffset.UtcNow.ToUnixTimeMilliseconds();
        var from = now - 7 * 24 * 3600_000L;

        try
        {
            var reply = await _client.GetSignalHistoryAsync(from, now, StrategyFilter, limit);
            Signals.Clear();
            foreach (var item in reply.Items)
                Signals.Add(new SignalHistoryRow(item));
            StatusMessage = $"Loaded {reply.Items.Count} signals";
        }
        catch (System.Exception ex)
        {
            StatusMessage = $"Error: {ex.Message}";
        }
    }
}

public sealed class SignalHistoryRow
{
    public string Id { get; }
    public string Timestamp { get; }
    public string Strategy { get; }
    public string GrossBps { get; }
    public string NetBps { get; }
    public string Executed { get; }

    public SignalHistoryRow(SignalHistoryReply.Types.SignalItem item)
    {
        Id = item.Id;
        Timestamp = DateTimeOffset
            .FromUnixTimeMilliseconds(item.TimestampUnixMs)
            .ToString("MM-dd HH:mm:ss");
        Strategy = item.Strategy;
        GrossBps = $"{item.GrossBps:F1}";
        NetBps = $"{item.NetBps:F1}";
        Executed = item.Executed ? "Yes" : "No";
    }
}
