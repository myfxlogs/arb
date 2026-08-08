using System.Collections.ObjectModel;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using ArbDesk.Dashboard;
using ArbDesk.Services;

namespace ArbDesk.ViewModels;

public partial class AdminViewModel : ObservableObject
{
    DashboardClient? _client;

    public ObservableCollection<StrategyStatusRow> Strategies { get; } = new();

    [ObservableProperty] string _statusMessage = "";

    public void Initialize(DashboardClient client) => _client = client;

    [RelayCommand]
    async Task LoadStrategies()
    {
        if (_client == null) return;
        try
        {
            var reply = await _client.GetStrategyStatusAsync();
            Strategies.Clear();
            foreach (var item in reply.Items)
                Strategies.Add(new StrategyStatusRow(item));
            StatusMessage = $"Loaded {reply.Items.Count} strategies";
        }
        catch (System.Exception ex)
        {
            StatusMessage = $"Error: {ex.Message}";
        }
    }

    [RelayCommand]
    async Task Kill()
    {
        if (_client == null) return;
        try
        {
            var reply = await _client.KillAsync();
            StatusMessage = reply.Success
                ? $"Killed: {reply.PositionsClosed} positions closed, {reply.OrdersCancelled} orders cancelled"
                : $"Kill failed: {reply.Error}";
        }
        catch (System.Exception ex)
        {
            StatusMessage = $"Error: {ex.Message}";
        }
    }

    [RelayCommand]
    async Task Resume()
    {
        if (_client == null) return;
        try
        {
            var reply = await _client.ResumeAsync();
            StatusMessage = reply.Success ? "Resumed" : "Resume failed";
        }
        catch (System.Exception ex)
        {
            StatusMessage = $"Error: {ex.Message}";
        }
    }

    [RelayCommand]
    async Task Toggle(StrategyStatusRow? row)
    {
        if (_client == null || row == null) return;
        try
        {
            var reply = await _client.ToggleStrategyAsync(row.Name, !row.Enabled);
            if (reply.Success)
            {
                row.Enabled = !row.Enabled;
                StatusMessage = $"{row.Name} {(row.Enabled ? "enabled" : "disabled")}";
            }
            else
            {
                StatusMessage = $"Toggle failed: {reply.Error}";
            }
        }
        catch (System.Exception ex)
        {
            StatusMessage = $"Error: {ex.Message}";
        }
    }
}

public sealed class StrategyStatusRow : System.ComponentModel.INotifyPropertyChanged
{
    public string Name { get; }
    public bool CircuitBreakerOpen { get; }
    public string ConsecutiveLosses { get; }
    public string WindowPnl { get; }
    public string TradesToday { get; }
    public string PnlToday { get; }

    bool _enabled;
    public bool Enabled
    {
        get => _enabled;
        set { _enabled = value; OnPropertyChanged(); }
    }

    public StrategyStatusRow(StrategyStatusReply.Types.StrategyItem item)
    {
        Name = item.Name;
        _enabled = item.Enabled;
        CircuitBreakerOpen = item.CircuitBreakerOpen;
        ConsecutiveLosses = $"{item.ConsecutiveLosses}";
        WindowPnl = $"{item.WindowPnl:F2}";
        TradesToday = $"{item.TradesToday}";
        PnlToday = $"{item.PnlToday:F2}";
    }

    public event System.ComponentModel.PropertyChangedEventHandler? PropertyChanged;
    void OnPropertyChanged([System.Runtime.CompilerServices.CallerMemberName] string? name = null) =>
        PropertyChanged?.Invoke(this, new System.ComponentModel.PropertyChangedEventArgs(name));
}
