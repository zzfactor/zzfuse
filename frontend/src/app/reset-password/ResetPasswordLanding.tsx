"use client";

import { useState, useSyncExternalStore } from "react";
import RequestResetForm from "./RequestResetForm";
import ResetPasswordForm from "./[token]/ResetPasswordForm";

function createFragmentTokenStore() {
    let token = "";
    return {
        snapshot: () => token,
        subscribe: (notify: () => void) => {
            const capture = () => {
                const value = new URLSearchParams(window.location.hash.slice(1)).get("token");
                if (!value) return;
                token = value;
                window.history.replaceState(window.history.state, "", window.location.pathname + window.location.search);
                notify();
            };
            window.addEventListener("hashchange", capture);
            capture();
            return () => window.removeEventListener("hashchange", capture);
        },
    };
}

export default function ResetPasswordLanding() {
    const [store] = useState(createFragmentTokenStore);
    const token = useSyncExternalStore(store.subscribe, store.snapshot, () => "");
    return token ? <ResetPasswordForm token={token} /> : <RequestResetForm />;
}
