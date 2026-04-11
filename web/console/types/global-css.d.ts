// TypeScript 6 removed the implicit allowance for side-effect imports of
// style assets. Declare the extensions we use so `import "./foo.css"` and
// side-effect imports from third-party packages type-check cleanly.

declare module "*.css";
declare module "*.scss";
