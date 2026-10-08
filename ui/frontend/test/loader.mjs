// A resolve hook that points the page's import of the Wails runtime, which the
// app serves at /wails/runtime.js, at a stub, so page modules load under Node
export async function resolve(specifier, context, next) {
  if (specifier === "/wails/runtime.js") {
    return { url: new URL("./runtime-stub.mjs", import.meta.url).href, shortCircuit: true };
  }
  return next(specifier, context);
}
