"use client";
import { useEffect, useRef } from "react";
import "swagger-ui-dist/swagger-ui.css";

export default function SwaggerDocs() {
    const containerRef = useRef<HTMLDivElement>(null);
    useEffect(() => {
        let cancelled = false;
        const container = containerRef.current;
        import("swagger-ui-dist/swagger-ui-bundle").then(({ default: SwaggerUI }) => {
            if (!cancelled && container) SwaggerUI({ domNode: container, url: "/api/backend-openapi", layout: "BaseLayout" });
        });
        return () => { cancelled = true; container?.replaceChildren(); };
    }, []);
    return <div ref={containerRef} />;
}
