"use client";
import Header from "../components/Header";
import Hero from "../components/Hero";
import MarkdownContent from "../components/MarkdownContent";
import { useSession } from "next-auth/react";
import { useRouter } from "next/navigation";
import { useEffect } from "react";

export default function HomePage() {
  const { status } = useSession();
  const router = useRouter();

  useEffect(() => {
    if (status === "authenticated") {
      router.replace("/dashboard");
    }
  }, [status, router]);

  if (status === "authenticated") {
    return null;
  }

  return (
    <div className="min-h-screen bg-background text-foreground">
      <Header />
      <main className="flex flex-col items-center justify-center px-4 pt-12 pb-24">
        <Hero />
        <div className="w-full max-w-3xl mt-16 bg-card text-card-foreground rounded-xl shadow-lg p-8">
          <MarkdownContent />
        </div>
      </main>
    </div>
  );
}
