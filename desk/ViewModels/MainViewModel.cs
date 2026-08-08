using System.Collections.ObjectModel;
using System.Threading;
using System.Threading.Tasks;
using System.Windows;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using ArbDesk.Dashboard;
using ArbDesk.Services;

namespace ArbDesk.ViewModels;

public partial class MainViewModel : ObservableObject
{
    DashboardClient? _client;
    CancellationTokenSource _cts = new();

    public TradingViewModel TradingVM { get; } = new();
    public HistoryViewModel HistoryVM { get; } = new();
    public AdminViewModel AdminVM { get; } = new();

    public ObservableCollection<OpportunityRow> Opportunities { get; } = new();
    public ObservableCollection<MatrixRow> MatrixRows { get; } = new();
    public ObservableCollection<PositionRow> Positions { get; } = new();

    [ObservableProperty]
    OpportunityRow? _selectedOpportunity;

    [ObservableProperty]
    string _connectionStatus = "Connecting...";

    [ObservableProperty]
    string _matrixTimestamp = "";

    [ObservableProperty]
    string _positionsTimestamp = "";

    public async Task InitializeAsync()
    {
        _client = App.Current.Dashboard;
        ConnectionStatus = "Connected";
        TradingVM.Initialize(_client);
        HistoryVM.Initialize(_client);
        AdminVM.Initialize(_client);
        _ = StartOpportunityStream(_cts.Token);
        _ = StartMatrixStream(_cts.Token);
        _ = StartPositionsStream(_cts.Token);
        await Task.CompletedTask;
    }

    async Task StartOpportunityStream(CancellationToken ct)
    {
        try
        {
            await foreach (var ev in _client!.OpportunityStream(ct))
            {
                Application.Current.Dispatcher.Invoke(() =>
                {
                    switch (ev.Action)
                    {
                        case OpportunityAction.Pushed:
                        case OpportunityAction.Updated:
                            InsertOrUpdate(ev.Opp);
                            break;
                        case OpportunityAction.Expired:
                        case OpportunityAction.Failed:
                            RemoveById(ev.Id);
                            break;
                    }
                });
            }
        }
        catch (System.OperationCanceledException) { }
        catch (System.Exception ex)
        {
            ConnectionStatus = $"Stream error: {ex.Message}";
        }
    }

    void InsertOrUpdate(Opportunity opp)
    {
        var existing = Opportunities.FirstOrDefault(o => o.Id == opp.Id);
        if (existing != null)
        {
            existing.UpdateFrom(opp);
        }
        else
        {
            var row = new OpportunityRow(opp);
            Opportunities.Insert(0, row);
        }
    }

    void RemoveById(string id)
    {
        var item = Opportunities.FirstOrDefault(o => o.Id == id);
        if (item != null)
            Opportunities.Remove(item);
    }

    [RelayCommand]
    async Task Confirm(OpportunityRow? row)
    {
        if (row == null || !row.CanConfirm) return;

        try
        {
            var reply = await _client!.ConfirmOpportunityAsync(row.Id);
            if (reply.Accepted)
            {
                row.Status = "Confirmed";
            }
            else
            {
                row.Status = $"Rejected: {reply.Reason}";
            }
        }
        catch (System.Exception ex)
        {
            row.Status = $"Error: {ex.Message}";
        }
    }

    async Task StartMatrixStream(CancellationToken ct)
    {
        try
        {
            await foreach (var reply in _client!.SpreadMatrixStream(ct))
            {
                Application.Current.Dispatcher.Invoke(() =>
                {
                    MatrixRows.Clear();
                    foreach (var row in reply.Rows)
                        MatrixRows.Add(new MatrixRow(row));
                    MatrixTimestamp = DateTimeOffset
                        .FromUnixTimeMilliseconds(reply.TimestampUnixMs)
                        .ToString("HH:mm:ss");
                });
            }
        }
        catch (System.OperationCanceledException) { }
        catch (System.Exception) { }
    }

    async Task StartPositionsStream(CancellationToken ct)
    {
        try
        {
            await foreach (var reply in _client!.PositionWatchStream(ct))
            {
                Application.Current.Dispatcher.Invoke(() =>
                {
                    Positions.Clear();
                    foreach (var bp in reply.BrokerPositions)
                        foreach (var pos in bp.Positions)
                            Positions.Add(new PositionRow(bp.BrokerName, pos));
                    PositionsTimestamp = DateTimeOffset
                        .FromUnixTimeMilliseconds(reply.TimestampUnixMs)
                        .ToString("HH:mm:ss");
                });
            }
        }
        catch (System.OperationCanceledException) { }
        catch (System.Exception) { }
    }

    public void Shutdown() => _cts.Cancel();
}
