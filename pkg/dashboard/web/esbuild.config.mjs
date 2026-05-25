import * as esbuild from "esbuild";

const watch = process.argv.includes("--watch");

const opts = {
  entryPoints: ["src/main.tsx"],
  bundle: true,
  outfile: "dist/bundle.js",
  format: "iife",
  target: ["es2020"],
  jsx: "automatic",
  minify: !watch,
  sourcemap: watch,
  loader: { ".tsx": "tsx", ".ts": "ts" },
  define: {
    "process.env.NODE_ENV": watch ? '"development"' : '"production"',
  },
  logLevel: "info",
};

if (watch) {
  const ctx = await esbuild.context(opts);
  await ctx.watch();
  console.log("esbuild watching...");
} else {
  await esbuild.build(opts);
}
