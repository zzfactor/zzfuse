"use client";
import { errorMessage } from "@/lib/errors";
import { useState, useEffect, useRef } from "react";
import { useParams, useRouter } from "next/navigation";
import { signOut } from "next-auth/react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import { Label } from "@/components/ui/label";
import { toast } from "react-hot-toast";

export default function ResetPasswordForm({ token: suppliedToken }: { token?: string } = {}) {
    const params = useParams();
    const token = suppliedToken || (params?.token ? (Array.isArray(params.token) ? params.token[0] : params.token) : undefined);
    const router = useRouter();
    const [password, setPassword] = useState("");
    const [confirm, setConfirm] = useState("");
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState("");
    const isMounted = useRef(true);
    const timeoutRef = useRef<NodeJS.Timeout | null>(null);

    useEffect(() => {
        isMounted.current = true;
        return () => {
            isMounted.current = false;
            if (timeoutRef.current) clearTimeout(timeoutRef.current);
        };
    }, []);

    // Fallback UI if token is missing
    if (!token) {
        return <div style={{ color: 'red' }}>Token is missing from URL!</div>;
    }

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!isMounted.current) return;
        setError("");
        if (password.length < 8) {
            setError("Password must be at least 8 characters.");
            return;
        }
        if (password !== confirm) {
            setError("Passwords do not match.");
            return;
        }
        setLoading(true);
        try {
            const res = await fetch("/api/reset-password", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ token, password }),
            });
            let data: { error?: string } = {};
            try {
                data = await res.json();
            } catch { /* Use status text when the response is not JSON. */ }
            if (!res.ok) {
                throw new Error(data?.error || res.statusText || "Failed to reset password");
            }
            await signOut({ redirect: false, callbackUrl: "/login" });
            if (isMounted.current) {
                toast.success("Password reset successful. You can now log in.");
                timeoutRef.current = setTimeout(() => {
                    if (isMounted.current) router.push("/login");
                }, 2000);
            }
        } catch (err) {
            if (isMounted.current) setError(errorMessage(err) || "An unexpected error occurred.");
        } finally {
            if (isMounted.current) setLoading(false);
        }
    };

    return (
        <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-gray-50 to-blue-100 dark:from-background dark:to-muted/50 font-sans">
            <div className="w-full max-w-md bg-white/90 dark:bg-background/80 rounded-2xl shadow-2xl p-8 backdrop-blur-md">
                <h1 className="text-2xl font-bold mb-4 text-center">Set New Password</h1>
                <form onSubmit={handleSubmit} className="space-y-5">
                    <div>
                        <Label htmlFor="password" className="block text-base font-medium mb-1">New Password</Label>
                        <Input
                            id="password"
                            type="password"
                            placeholder="Enter new password"
                            value={password}
                            onChange={e => setPassword(e.target.value)}
                            required
                        />
                    </div>
                    <div>
                        <Label htmlFor="confirm" className="block text-base font-medium mb-1">Confirm Password</Label>
                        <Input
                            id="confirm"
                            type="password"
                            placeholder="Confirm new password"
                            value={confirm}
                            onChange={e => setConfirm(e.target.value)}
                            required
                        />
                    </div>
                    {error && (
                        <Alert variant="destructive">
                            <AlertTitle>Error</AlertTitle>
                            <AlertDescription>{error}</AlertDescription>
                        </Alert>
                    )}
                    <Button type="submit" className="w-full font-semibold text-base py-2" disabled={loading}>
                        {loading ? "Resetting..." : "Set New Password"}
                    </Button>
                </form>
            </div>
        </div>
    );
}
