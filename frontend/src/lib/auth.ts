import CredentialsProvider from "next-auth/providers/credentials";
import type { NextAuthOptions } from "next-auth";
import type { BackendUser } from "@/types/next-auth";
import { backendURL } from "@/lib/backend";

export const authOptions: NextAuthOptions = {
    providers: [
        CredentialsProvider({
            name: "Credentials",
            credentials: {
                email: { label: "Email", type: "email" },
                password: { label: "Password", type: "password" },
            },
            async authorize(credentials) {
                if (!credentials?.email || !credentials.password) return null;
                const res = await fetch(backendURL("/api/v1/auth/login"), {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({ username: credentials.email, password: credentials.password }),
                    signal: AbortSignal.timeout(15_000),
                });
                if (!res.ok) return null;
                const data = await res.json() as { token?: string; user?: BackendUser; roles?: string[] };
                if (!data.token || !data.user) return null;
                return {
                    id: String(data.user.id),
                    email: data.user.email,
                    name: data.user.name || data.user.username,
                    token: data.token,
                    backendUser: data.user,
                    roles: data.roles || [],
                };
            },
        }),
    ],
    session: { strategy: "jwt" },
    events: {
        async signOut(message) {
            if (!("token" in message) || typeof message.token?.jwt !== "string") return;
            try {
                const response = await fetch(backendURL("/api/v1/auth/logout"), {
                    method: "POST",
                    headers: { Authorization: `Bearer ${message.token.jwt}` },
                    signal: AbortSignal.timeout(5_000),
                    cache: "no-store",
                });
                if (!response.ok && response.status !== 401) {
                    console.error("Backend session revocation failed", response.status);
                }
            } catch {
                console.error("Backend session revocation was unavailable");
            }
        },
    },
    callbacks: {
        async jwt({ token, user }) {
            if (user?.token) {
                token.jwt = user.token;
                token.backendUser = user.backendUser;
                token.roles = user.roles;
            }
            return token;
        },
        async session({ session, token }) {
            session.jwt = token.jwt;
            if (token.backendUser) session.user = token.backendUser;
            session.roles = token.roles;
            return session;
        },
    },
};
