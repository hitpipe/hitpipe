/**
 * Next16.4.2 declarations refer to these standard URLPattern aliases, while
 * TypeScript5.9 DOM and @types/node24 export only the corresponding module
 * interfaces. This type-only bridge keeps strict library checking enabled.
 * It changes no runtime and is removed together with a reviewed TS upgrade.
 */
import type { URLPatternInit as NodeURLPatternInit, URLPatternOptions as NodeURLPatternOptions } from 'node:url';
declare global {
  type URLPatternInput = string | NodeURLPatternInit;
  interface URLPatternOptions extends NodeURLPatternOptions {}
}
export {};
