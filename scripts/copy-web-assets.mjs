import { copyFileSync, mkdirSync } from "node:fs";
import { dirname } from "node:path";

const copies = [
  [
    "node_modules/@ibm/plex/IBM-Plex-Sans/fonts/complete/woff2/IBMPlexSans-Regular.woff2",
    "internal/web/static/fonts/ibm-plex/IBMPlexSans-Regular.woff2",
  ],
  [
    "node_modules/@ibm/plex/IBM-Plex-Sans/fonts/complete/woff2/IBMPlexSans-Medium.woff2",
    "internal/web/static/fonts/ibm-plex/IBMPlexSans-Medium.woff2",
  ],
  [
    "node_modules/@ibm/plex/IBM-Plex-Sans/fonts/complete/woff2/IBMPlexSans-SemiBold.woff2",
    "internal/web/static/fonts/ibm-plex/IBMPlexSans-SemiBold.woff2",
  ],
  [
    "node_modules/@ibm/plex/IBM-Plex-Mono/fonts/complete/woff2/IBMPlexMono-Regular.woff2",
    "internal/web/static/fonts/ibm-plex/IBMPlexMono-Regular.woff2",
  ],
  [
    "node_modules/@ibm/plex/IBM-Plex-Mono/fonts/complete/woff2/IBMPlexMono-Medium.woff2",
    "internal/web/static/fonts/ibm-plex/IBMPlexMono-Medium.woff2",
  ],
  [
    "node_modules/@ibm/plex/IBM-Plex-Mono/fonts/complete/woff2/IBMPlexMono-SemiBold.woff2",
    "internal/web/static/fonts/ibm-plex/IBMPlexMono-SemiBold.woff2",
  ],
  [
    "node_modules/@ibm/plex/LICENSE.txt",
    "third_party/fonts/ibm-plex/OFL.txt",
  ],
];

for (const [from, to] of copies) {
  mkdirSync(dirname(to), { recursive: true });
  copyFileSync(from, to);
}
