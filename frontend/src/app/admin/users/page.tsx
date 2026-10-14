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
};

export default function UserManagementPage() {
    const { data: session, status } = useSession();
    const [users, setUsers] = useState<User[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState("");
    const [resettingId, setResettingId] = useState<number | null>(null);

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
                // Map role from user object (if roles is array, take first; else fallback)
                setUsers((data as User[]).map((u) => ({
                    id: u.id,
                    email: u.email,
                    role: u.role || (u.roles ? u.roles[0] : undefined) || "user"
                })));
            } catch (err) {
                setError(errorMessage(err));
            } finally {
                setLoading(false);
            }
        };
        if (session?.jwt && isAdmin(session)) fetchUsers();
    }, [session]);

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
                                    <th className="px-2 py-1 border">Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                {loading ? (
                                    <tr>
                                        <td colSpan={3} className="text-center py-4">
                                            Loading...
                                        </td>
                                    </tr>
                                ) : (
                                    users.map((user) => (
                                        <tr key={user.id}>
                                            {/* <td className="px-2 py-1 border">{user.id}</td> */}
                                            <td className="px-2 py-1 border">{user.email}</td>
                                            <td className="px-2 py-1 border">{user.role === "admin" ? "Admin" : "User"}</td>
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
