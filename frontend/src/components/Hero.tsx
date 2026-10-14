export default function Hero() {
    return (
        <section className="w-full flex flex-col items-center text-center mt-12">
            <span className="inline-flex items-center gap-2 px-4 py-1 mb-4 rounded-full bg-neutral-100 dark:bg-neutral-800 text-sm font-medium text-neutral-700 dark:text-neutral-200 shadow-sm">
                <span className="font-semibold">OpenTelemetry</span>
                <span>Collect, store, and explore telemetry</span>
            </span>
            <h1 className="text-5xl md:text-6xl font-extrabold tracking-tight mb-6 max-w-3xl">
                zzfuse<br />Observability for your applications
            </h1>
            <p className="text-lg md:text-xl text-neutral-600 dark:text-neutral-300 mb-8 max-w-2xl">
                Bring logs, traces, metrics, and profiles together with an OpenTelemetry collector and PostgreSQL storage.
            </p>
            <div className="flex gap-4 justify-center">
                <a href="/register" className="px-6 py-3 rounded-lg bg-black text-white font-semibold text-lg shadow hover:bg-neutral-900 transition">Get Started →</a>
                <a href="https://github.com/zzfactor/zzfuse" className="px-6 py-3 rounded-lg bg-neutral-200 dark:bg-neutral-700 text-black dark:text-white font-semibold text-lg shadow hover:bg-neutral-300 dark:hover:bg-neutral-600 transition">View Repository</a>
            </div>
        </section>
    );
}
