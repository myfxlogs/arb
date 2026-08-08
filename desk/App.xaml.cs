using System.Windows;

namespace ArbDesk;

public partial class App : Application
{
    public static new App Current => (App)Application.Current;

    public Services.DashboardClient Dashboard { get; private set; } = null!;

    protected override void OnStartup(StartupEventArgs e)
    {
        base.OnStartup(e);

        var coreAddress = "http://localhost:50051";
        for (int i = 0; i < e.Args.Length - 1; i++)
        {
            if (e.Args[i] == "--core" && i + 1 < e.Args.Length)
                coreAddress = e.Args[i + 1];
        }

        Dashboard = new Services.DashboardClient(coreAddress);
    }

    protected override void OnExit(ExitEventArgs e)
    {
        Dashboard?.Dispose();
        base.OnExit(e);
    }
}
