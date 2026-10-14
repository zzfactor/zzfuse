"use client";
import { useState } from "react";
import { signIn } from "next-auth/react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import { Label } from "@/components/ui/label";
import { Card, CardContent } from "@/components/ui/card";

export default function LoginPage() {
    const [email, setEmail] = useState("");
    const [password, setPassword] = useState("");
    const [error, setError] = useState("");
    const [loading, setLoading] = useState(false);
    const router = useRouter();

    const handleLogin = async (e: React.FormEvent) => {
        e.preventDefault();
        setError("");
        setLoading(true);
        const res = await signIn("credentials", {
            redirect: false,
            email,
            password,
        });
        setLoading(false);
        if (res?.error) setError(res.error);
        else router.push("/dashboard");
    };

    return (
        <div className="flex min-h-screen items-center justify-center bg-background text-foreground">
            <Card className="w-full max-w-md p-0 rounded-2xl shadow-lg border-none bg-card text-card-foreground">
                <CardContent className="py-10 px-8 flex flex-col items-center">
                    <div className="mb-8 text-2xl font-bold tracking-tight text-center select-none">
                        zzfuse
                    </div>
                    <h1 className="text-xl font-semibold text-center mb-2">Sign in to your account</h1>
                    <p className="text-sm text-muted-foreground text-center mb-8">
                        Welcome back! Please enter your details
                    </p>
                    <form onSubmit={handleLogin} className="w-full flex flex-col gap-4">
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
                                autoComplete="current-password"
                            />
                        </div>
                        <div className="text-right mb-2">
                            <Link href="/reset-password" className="text-xs text-primary hover:underline">
                                Password forgotten?
                            </Link>
                        </div>
                        {error && (
                            <Alert variant="destructive">
                                <AlertTitle>Error</AlertTitle>
                                <AlertDescription>{error}</AlertDescription>
                            </Alert>
                        )}
                        <Button type="submit" className="w-full font-semibold text-base py-2 mt-2" disabled={loading}>
                            Sign in with Email
                        </Button>
                    </form>
                    <div className="mt-8 text-center w-full">
                        <span className="text-sm text-gray-500">Do not have an account yet? </span>
                        <Link href="/register" className="text-sm text-primary hover:underline">
                            Register
                        </Link>
                    </div>
                </CardContent>
            </Card>
        </div>
    );
}
