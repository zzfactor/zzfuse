"use client";
import { errorMessage } from "@/lib/errors";
import { useSession } from "next-auth/react";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";

type User = {
    id: number;
    email: string;
    name?: string;
    created_at?: string;
};

export default function AdminPage() {
    const { data: session } = useSession();
    const [users, setUsers] = useState<User[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState("");

    // Fetch users
    useEffect(() => {
        const fetchUsers = async () => {
            setLoading(true);
            setError("");
            try {
                const res = await fetch("/api/users", {
                    headers: {
                        Authorization: `Bearer ${session?.jwt}`,
                    },
                });
                if (!res.ok) throw new Error("Failed to fetch users");
                setUsers(await res.json());
            } catch (err) {
                setError(errorMessage(err));
            } finally {
                setLoading(false);
            }
        };
        if (session?.jwt) fetchUsers();
    }, [session?.jwt]);

    // Delete user
    const handleDelete = async (id: number) => {
        if (!confirm("Are you sure you want to delete this user?")) return;
        setError("");
        try {
            const res = await fetch(`/api/users/${id}`, {
                method: "DELETE",
                headers: { Authorization: `Bearer ${session?.jwt}` },
            });
            if (!res.ok) {
                const data = await res.json();
                throw new Error(data?.error || "Failed to delete user");
            }
            setUsers(users.filter((u) => u.id !== id));
        } catch (err) {
            setError(errorMessage(err));
        }
    };

    return (
        <div className="min-h-screen bg-background text-foreground flex flex-col">
            {/* Main Content */}
            <main className="flex-1 p-8">
                <section className="mb-8">
                    <h2 className="text-lg font-semibold mb-2">User Management</h2>
                    {error && (
                        <Alert variant="destructive" className="mb-4">
                            <AlertTitle>Error</AlertTitle>
                            <AlertDescription>{error}</AlertDescription>
                        </Alert>
                    )}
                    <div className="overflow-x-auto">
                        <table className="min-w-full border">
                            <thead>
                                <tr>
                                    <th className="px-2 py-1 border">ID</th>
                                    <th className="px-2 py-1 border">Email</th>
                                    <th className="px-2 py-1 border">Name</th>
                                    <th className="px-2 py-1 border">Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                {loading ? (
                                    <tr>
                                        <td colSpan={4} className="text-center py-4">
                                            Loading...
                                        </td>
                                    </tr>
                                ) : (
                                    users.map((user) => (
                                        <tr key={user.id}>
                                            <td className="px-2 py-1 border">{user.id}</td>
                                            <td className="px-2 py-1 border">{user.email}</td>
                                            <td className="px-2 py-1 border">{user.name}</td>
                                            <td className="px-2 py-1 border">
                                                <Button
                                                    variant="destructive"
                                                    size="sm"
                                                    onClick={() => handleDelete(user.id)}
                                                >
                                                    Delete
                                                </Button>
                                            </td>
                                        </tr>
                                    ))
                                )}
                            </tbody>
                        </table>
                    </div>
                </section>
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
