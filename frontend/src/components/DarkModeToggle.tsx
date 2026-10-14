import { useMounted } from "@/lib/use-mounted";
import { useTheme } from "next-themes";
import { SunIcon, MoonIcon, ComputerDesktopIcon } from "@heroicons/react/24/outline";

export function DarkModeToggle() {
    const { theme, setTheme, resolvedTheme } = useTheme();
    const mounted = useMounted();
    if (!mounted) return null;
    const isLight = resolvedTheme === "light";
    const isDark = resolvedTheme === "dark";
    const isSystem = theme === "system" && !isLight && !isDark;
    return (
        <div className="flex items-center gap-2">
            <button
                aria-label="Set light mode"
                className={`p-2 rounded hover:bg-neutral-100 dark:hover:bg-neutral-800 ${isLight ? "bg-primary/10 ring-2 ring-primary" : ""}`}
                onClick={() => setTheme("light")}
            >
                <SunIcon className="w-5 h-5" />
            </button>
            <button
                aria-label="Set dark mode"
                className={`p-2 rounded hover:bg-neutral-100 dark:hover:bg-neutral-800 ${isDark ? "bg-primary/10 ring-2 ring-primary" : ""}`}
                onClick={() => setTheme("dark")}
            >
                <MoonIcon className="w-5 h-5" />
            </button>
            <button
                aria-label="Set system mode"
                className={`p-2 rounded hover:bg-neutral-100 dark:hover:bg-neutral-800 ${isSystem ? "bg-primary/10 ring-2 ring-primary" : ""}`}
                onClick={() => setTheme("system")}
            >
                <ComputerDesktopIcon className="w-5 h-5" />
            </button>
        </div>
    );
}
