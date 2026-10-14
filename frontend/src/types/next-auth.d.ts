import type { DefaultSession } from "next-auth";

export type BackendUser = {
    id: number;
    email: string;
    username?: string;
    name?: string;
    role?: string;
    roles?: string[];
};

declare module "next-auth" {
    interface Session {
        jwt?: string;
        roles?: string[];
        user: BackendUser & DefaultSession["user"];
    }
    interface User {
        token?: string;
        backendUser?: BackendUser;
        roles?: string[];
    }
}

declare module "next-auth/jwt" {
    interface JWT {
        jwt?: string;
        backendUser?: BackendUser;
        roles?: string[];
    }
}
