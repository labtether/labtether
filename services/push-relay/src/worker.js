// @ts-check
import { handleRequest } from "./index.js";
import { EGRESS_PATH, forwardPush, handleEgress, egressConfigured } from "./egress.js";
import { errorResponse } from "./http.js";

export default {
  /**
   * @param {Request} request
   * @param {import("./index.js").Env & import("./egress.js").EgressEnv} env
   * @param {import("./index.js").ExecutionContextLike} ctx
   */
  fetch(request, env, ctx) {
    const url = new URL(request.url);
    if (url.pathname === EGRESS_PATH) return handleEgress(request, env);
    const direct = url.origin === env.APNS_EGRESS_ORIGIN;
    if (!direct && url.pathname === "/v1/push" && !egressConfigured(env)) {
      return errorResponse(503, "not_configured");
    }
    return handleRequest(request, env, ctx, {
      now: () => Date.now(),
      fetch: (input, init) => direct ? fetch(input, init) : forwardPush(input, init, env),
    });
  },
};
