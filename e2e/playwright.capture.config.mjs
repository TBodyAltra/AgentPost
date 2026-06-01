import base from "./playwright.config.mjs";

/** Config for regenerating docs/images/dashboard.png (not run in CI dashboard job). */
export default {
  ...base,
  testIgnore: [],
  testMatch: "capture-pages-dashboard.spec.mjs",
};
