"use client";
import { errorMessage } from "@/lib/errors";
import { useSession } from "next-auth/react";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import { isAdmin } from "@/lib/roles";

type User = {
    id: number;
    email: string;
    role?: string;
    roles?: string[];
    schemas: string[];
    disabled: boolean;
};

export default function UserManagementPage() {
    const { data: session, status } = useSession();
    const [users, setUsers] = useState<User[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState("");
    const [resettingId, setResettingId] = useState<number | null>(null);
    const [savingId, setSavingId] = useState<number | null>(null);
    const [schemaDrafts, setSchemaDrafts] = useState<Record<number, string>>({});

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
                const data = await res.json();
                const loaded = data as User[];
                setUsers(loaded);
                setSchemaDrafts(Object.fromEntries(loaded.map(user => [user.id, (user.schemas || []).join(", ")])));
            } catch (err) {
                setError(errorMessage(err));
            } finally {
                setLoading(false);
            }
        };
        if (session?.jwt && isAdmin(session)) fetchUsers();
    }, [session]);

    const updateAccess = async (user: User, change: { schemas: string[] } | { disabled: boolean } | { revoke_sessions: true }) => {
        setSavingId(user.id);
        setError("");
        try {
            const res = await fetch(`/api/users/${user.id}/access`, {
                method: "POST",
                headers: { "Content-Type": "application/json", Authorization: `Bearer ${session?.jwt}` },
                body: JSON.stringify(change),
            });
            if (!res.ok) {
                const data = await res.json();
                throw new Error(data?.error || "Failed to update access");
            }
            setUsers(current => current.map(row => row.id === user.id ? { ...row, ...change } : row));
        } catch (err) {
            setError(errorMessage(err));
        } finally {
            setSavingId(null);
        }
    };

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

    const handleResetPassword = async (id: number, email: string) => {
        setResettingId(id);
        setError("");
        try {
            const res = await fetch(`/api/request-password-reset`, {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ email }),
            });
            const data = await res.json();
            if (!res.ok) {
                throw new Error(data?.error || "Failed to request password reset");
            }
            alert("Password reset requested.");
        } catch (err) {
            setError(errorMessage(err));
        } finally {
            setResettingId(null);
        }
    };

    if (status === "loading") return <div className="flex min-h-screen items-center justify-center">Loading...</div>;
    if (!session || !isAdmin(session)) return <div className="flex min-h-screen items-center justify-center text-red-500 font-semibold">Unauthorized: Admins only</div>;

    return (
        <div className="min-h-screen bg-background text-foreground flex flex-col">
            <main className="flex-1 p-8">
                <section className="mb-8">
                    <h2 className="text-lg font-semibold mb-2">Users</h2>
                    <p className="mb-4 text-sm text-muted-foreground">Users need explicit schema grants. An empty list removes all schema access. Administrators can access all schemas.</p>
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
                                    {/* <th className="px-2 py-1 border">ID</th> */}
                                    <th className="px-2 py-1 border">Email</th>
                                    <th className="px-2 py-1 border">Role</th>
                                    <th className="px-2 py-1 border">Schema access</th>
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
                                            {/* <td className="px-2 py-1 border">{user.id}</td> */}
                                            <td className="px-2 py-1 border">{user.email}</td>
                                            <td className="px-2 py-1 border">{user.roles?.includes("admin") ? "Admin" : "User"}{user.disabled ? " (disabled)" : ""}</td>
                                            <td className="px-2 py-1 border">
                                                <input
                                                    aria-label={`Schemas for ${user.email}`}
                                                    className="rounded border px-2 py-1"
                                                    placeholder="otel, tenant_a"
                                                    value={schemaDrafts[user.id] || ""}
                                                    onChange={event => setSchemaDrafts(current => ({ ...current, [user.id]: event.target.value }))}
                                                />
                                                <Button size="sm" variant="outline" className="ml-2" disabled={savingId === user.id}
                                                    onClick={() => updateAccess(user, { schemas: (schemaDrafts[user.id] || "").split(",").map(value => value.trim()).filter(Boolean) })}>
                                                    Save grants
                                                </Button>
                                            </td>
                                            <td className="px-2 py-1 border flex gap-2">
                                                <Button
                                                    variant="destructive"
                                                    size="sm"
                                                    onClick={() => handleDelete(user.id)}
                                                    disabled={users.length === 1}
                                                >
                                                    Delete
                                                </Button>
                                                <Button
                                                    variant="outline"
                                                    size="sm"
                                                    onClick={() => handleResetPassword(user.id, user.email)}
                                                    disabled={resettingId === user.id}
                                                >
                                                    {resettingId === user.id ? "Resetting..." : "Reset Password"}
                                                </Button>
                                                <Button variant="outline" size="sm" disabled={savingId === user.id}
                                                    onClick={() => updateAccess(user, { revoke_sessions: true })}>Revoke sessions</Button>
                                                <Button variant="outline" size="sm" disabled={savingId === user.id}
                                                    onClick={() => updateAccess(user, { disabled: !user.disabled })}>{user.disabled ? "Enable" : "Disable"}</Button>
                                            </td>
                                        </tr>
                                    ))
                                )}
                            </tbody>
                        </table>
                    </div>
                </section>
            </main>
        </div>
    );
}
