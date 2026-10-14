import { redirect } from "next/navigation";
import ResetPasswordLanding from "./ResetPasswordLanding";

export default async function ResetPasswordPage({
    searchParams,
}: {
    searchParams: Promise<{ token?: string | string[] }>;
}) {
    const { token } = await searchParams;
    if (typeof token === "string" && token.trim()) {
        redirect(`/reset-password/${encodeURIComponent(token)}`);
    }
    return <ResetPasswordLanding />;
}
