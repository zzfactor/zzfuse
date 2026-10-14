"use client";
import SwaggerDocs from "@/components/SwaggerDocs";
import DashboardLayout from "../dashboard/layout";
import "../../styles/swagger-ui-custom.css";


export default function ApiDocsPage() {
    return (
        <DashboardLayout>
            <div className="w-full min-h-screen flex justify-center bg-background text-foreground font-sans antialiased overflow-x-auto py-8">
                <div className="w-full max-w-5xl px-4">
                    <SwaggerDocs />
                </div>
            </div>
        </DashboardLayout>
    );
}
