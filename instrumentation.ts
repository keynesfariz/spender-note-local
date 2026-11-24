export async function register() {
  if (process.env.NEXT_RUNTIME === "nodejs") {
    const { configureGoRouteStdout } = await import("./instrumentation-node")
    configureGoRouteStdout()
  }
}
