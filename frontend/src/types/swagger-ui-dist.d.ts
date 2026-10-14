declare module "swagger-ui-dist/swagger-ui-bundle" {
    const SwaggerUI: (options: { domNode: HTMLElement; url: string; layout: string }) => unknown;
    export default SwaggerUI;
}
