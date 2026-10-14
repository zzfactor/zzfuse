"use client";
import { errorMessage } from "@/lib/errors";
import { useState } from "react";
import Link from "next/link";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import { Label } from "@/components/ui/label";
import { Card, CardContent } from "@/components/ui/card";
import { signIn } from "next-auth/react";

export default function RegisterPage() {
    const [email, setEmail] = useState("");
    const [password, setPassword] = useState("");
    const [error, setError] = useState("");
    const [loading, setLoading] = useState(false);
    const [username, setUsername] = useState("");

    const handleRegister = async (e: React.FormEvent) => {
        e.preventDefault();
        setError("");
        setLoading(true);
        try {
            const res = await fetch("/api/register", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ username, email, password }),
            });
            const data = await res.json();
            if (!res.ok) throw new Error(data.error || "Registration failed");
            await signIn("credentials", {
                redirect: true,
                email,
                password,
                callbackUrl: "/"
            });
        } catch (err) {
            setError(errorMessage(err));
        } finally {
            setLoading(false);
        }
    };

    return (
        <div className="flex min-h-screen items-center justify-center bg-background text-foreground">
            <Card className="w-full max-w-md p-0 rounded-2xl shadow-lg border-none bg-card text-card-foreground">
                <CardContent className="py-10 px-8 flex flex-col items-center">
                    <div className="mb-8 text-2xl font-bold tracking-tight text-center select-none">
                        zzfuse
                    </div>
                    <h1 className="text-xl font-semibold text-center mb-2">Create your account</h1>
                    <p className="text-sm text-muted-foreground text-center mb-8">
                        Enter your details to register
                    </p>
                    <form onSubmit={handleRegister} className="w-full flex flex-col gap-4">
                        <div>
                            <Label htmlFor="username" className="text-sm font-medium mb-1 block">Username</Label>
                            <Input id="username" value={username} onChange={e => setUsername(e.target.value)} required minLength={3} maxLength={64} autoComplete="username" />
                        </div>
                        <div>
                            <Label htmlFor="email" className="text-sm font-medium mb-1 block">
                                Email Address
                            </Label>
                            <Input
                                id="email"
                                type="email"
                                placeholder="your@email.com"
                                value={email}
                                onChange={e => setEmail(e.target.value)}
                                required
                                autoComplete="email"
                            />
                        </div>
                        <div>
                            <Label htmlFor="password" className="text-sm font-medium mb-1 block">
                                Password
                            </Label>
                            <Input
                                id="password"
                                type="password"
                                placeholder=""
                                value={password}
                                onChange={e => setPassword(e.target.value)}
                                required
                                autoComplete="new-password"
                                minLength={8}
                            />
                        </div>
                        {error && (
                            <Alert variant="destructive">
                                <AlertTitle>Error</AlertTitle>
                                <AlertDescription>{error}</AlertDescription>
                            </Alert>
                        )}
                        <Button type="submit" className="w-full font-semibold text-base py-2 mt-2" disabled={loading}>
                            {loading ? "Registering..." : "Register"}
                        </Button>
                    </form>
                    <div className="mt-8 text-center w-full">
                        <span className="text-sm text-muted-foreground">Already have an account? </span>
                        <Link href="/login" className="text-sm text-primary hover:underline">
                            Sign in
                        </Link>
                    </div>
                </CardContent>
            </Card>
        </div>
    );
}
