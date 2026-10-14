"use client";
import { errorMessage } from "@/lib/errors";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import { Label } from "@/components/ui/label";

export default function RequestResetForm() {
    const [email, setEmail] = useState("");
    const [loading, setLoading] = useState(false);
    const [success, setSuccess] = useState("");
    const [error, setError] = useState("");

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        setLoading(true);
        setSuccess("");
        setError("");
        try {
            const res = await fetch("/api/reset-password", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ email }),
            });
            const data = await res.json();
            if (!res.ok) throw new Error(data.error || "Failed to send reset link");
            setSuccess("If an account with that email exists, a reset link has been sent.");
        } catch (err) {
            setError(errorMessage(err));
        } finally {
            setLoading(false);
        }
    };

    return (
        <div className="min-h-screen flex items-center justify-center bg-background text-foreground font-sans">
            <div className="w-full max-w-md bg-card text-card-foreground rounded-2xl shadow-2xl p-8 backdrop-blur-md">
                <h1 className="text-2xl font-bold mb-4 text-center">Reset Password</h1>
                <form onSubmit={handleSubmit} className="space-y-5">
                    <div>
                        <Label htmlFor="email" className="block text-base font-medium mb-1">Email</Label>
                        <Input
                            id="email"
                            type="email"
                            placeholder="Enter your email"
                            value={email}
                            onChange={e => setEmail(e.target.value)}
                            required
                        />
                    </div>
                    {success && (
                        <Alert variant="default">
                            <AlertTitle>Success</AlertTitle>
                            <AlertDescription>{success}</AlertDescription>
                        </Alert>
                    )}
                    {error && (
                        <Alert variant="destructive">
                            <AlertTitle>Error</AlertTitle>
                            <AlertDescription>{error}</AlertDescription>
                        </Alert>
                    )}
                    <Button type="submit" className="w-full font-semibold text-base py-2" disabled={loading}>
                        {loading ? "Sending..." : "Send Reset Link"}
                    </Button>
                </form>
            </div>
        </div>
    );
}
