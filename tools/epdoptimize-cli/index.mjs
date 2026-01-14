// Simple CLI to apply epdoptimize dithering with Spectra 6 defaults and map to device colors.
// Usage:
//   node index.mjs --input /path/in.png --output /path/out.png [--matrix floydSteinberg] [--serpentine true]
// Defaults:
//   palette: spectra6
//   deviceColors: spectra6
//   ditheringType: errorDiffusion
/* Defaults:
   palette: spectra6
   deviceColors: spectra6
   ditheringType: errorDiffusion
   errorDiffusionMatrix: floydSteinberg
   serpentine: false (matches demo defaults)
*/

import { createCanvas, loadImage } from 'canvas';
import {
  ditherImage,
  getDefaultPalettes,
  getDeviceColors,
  replaceColors,
} from 'epdoptimize';


// Basic arg parsing
function parseArgs(argv) {
  const args = {};
  for (let i = 2; i < argv.length; i++) {
    const token = argv[i];
    if (token.startsWith('--')) {
      const key = token.slice(2);
      const next = argv[i + 1];
      if (!next || next.startsWith('--')) {
        args[key] = true;
      } else {
        args[key] = next;
        i++;
      }
    }
  }
  return args;
}

async function main() {
  const args = parseArgs(process.argv);

  const inputPath = args.input;
  const outputPath = args.output;

  if (!inputPath || !outputPath) {
    console.error('Missing required arguments. Usage: node index.mjs --input /path/in.png --output /path/out.png [--matrix floydSteinberg] [--serpentine true]');
    process.exit(2);
  }

  // Defaults
  const paletteName = 'spectra6';
  const deviceName = 'spectra6';
  const ditheringType = 'errorDiffusion';
  const matrix = args.matrix || 'floydSteinberg';
  const serpentine = args.serpentine !== undefined ? args.serpentine === 'true' || args.serpentine === true : false;

  try {
    // Load image
    const img = await loadImage(inputPath);
    const width = img.width;
    const height = img.height;

    // Create canvases
    const inputCanvas = createCanvas(width, height);
    const inputCtx = inputCanvas.getContext('2d');
    inputCtx.drawImage(img, 0, 0);

    const ditheredCanvas = createCanvas(width, height);
    const deviceCanvas = createCanvas(width, height);

    // Prepare palette/device colors
    const paletteHex = getDefaultPalettes(paletteName);
    const deviceColors = getDeviceColors(deviceName);

    // Dither
    ditherImage(inputCanvas, ditheredCanvas, {
      ditheringType: ditheringType,
      errorDiffusionMatrix: matrix,
      serpentine: serpentine,
      palette: paletteName,
    });

    // Map to device colors
    replaceColors(ditheredCanvas, deviceCanvas, {
      originalColors: paletteHex,
      replaceColors: deviceColors,
    });

    // Write out PNG
    const outBuffer = deviceCanvas.toBuffer('image/png');
    const fs = await import('node:fs');
    await fs.promises.writeFile(outputPath, outBuffer);

    console.log(`Wrote optimized PNG to ${outputPath}`);
    process.exit(0);
  } catch (err) {
    console.error('Failed to optimize image:', err && err.stack ? err.stack : err);
    process.exit(1);
  }
}

main();
