"use client";

export default function DashboardPage() {
    return (
        <div className="min-h-screen bg-background text-foreground flex flex-col">
            <main className="flex-1 p-8">
                <section>
                    <h2 className="text-lg font-semibold mb-2">Welcome</h2>
                    <div className="bg-card p-4 rounded shadow">
                        Welcome to the Admin Section (Session Protected)
                    </div>
                </section>
            </main>
        </div>
    );
}
