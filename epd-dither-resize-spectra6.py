#!/usr/bin/env python3
"""
Script to generate BMP images used by waveshare's ESP32-S3-PhotoPainter

ESP32-S3-PhotoPainter: https://www.waveshare.com/wiki/ESP32-S3-PhotoPainter

Forked from waveshare's color tool: https://files.waveshare.com/wiki/common/ConverTo6c_bmp-7.3.zip
with changes:
- Rotate automatically.
- Adopted epdoptimize's real-world colors: https://github.com/Utzel-Butzel/epdoptimize

There are 3 output formats:
- output/dithered: Preview of the dithering result on computer.
- output/device: BMP images intended to be used on device that takes BMP.
  They look bad on computer, but should look regular on e-ink screens.
  For example, with the waveshare stock PhotoPainter firmware, you can copy
  the BMP files to the SD card.
- output/raw: Raw data (1 pixel = 4 bits, 2 pixels = 1 byte) to be used by
  low-level device functions. Requires need some coding skills to use they
  properly. They are 6x smaller than BMPs so they can reduce ESP32 processing
  time and memory usage, and are more reliable to transmit over Wi-Fi.

For the waveshare 13.3 inch SPECTRA6 e-paper display, try these flags:
    --size=1200x1600 --rotate-180
"""

import sys
import os.path
from PIL import Image, ImageOps
import argparse

# Create an ArgumentParser object
parser = argparse.ArgumentParser(description="Process some images.")

# Add orientation parameter
parser.add_argument("image_file", type=str, help="Input image file")
parser.add_argument(
    "--size",
    default="800x480",
    help="Output image size (width x height)",
)
parser.add_argument(
    "--rotate-180",
    action="store_true",
    default=False,
    help="Rotate 180 degree before processing.",
)
parser.add_argument(
    "--rotate-angle",
    type=int,
    choices=[90, 270],
    default=270,
    help="Rotate angle if orientation mismatches. 90 or 270.",
)
parser.add_argument(
    "--mode",
    choices=["scale", "cut"],
    default="scale",
    help="Image conversion mode (scale or cut)",
)
parser.add_argument(
    "--dither",
    type=int,
    choices=[Image.NONE, Image.FLOYDSTEINBERG],
    default=Image.FLOYDSTEINBERG,
    help="Image dithering algorithm (NONE(0) or FLOYDSTEINBERG(3))",
)

# Parse command line arguments
args = parser.parse_args()

# Get input parameter
input_filename = args.image_file
display_mode = args.mode
display_dither = Image.Dither(args.dither)
target_width, target_height = list(
    map(int, args.size.lower().replace("*", "x").split("x"))
)
angle = args.rotate_angle
rotate_180 = args.rotate_180

SPECTRA6_REAL_WORD_RGB = [
    (25, 30, 33),
    (232, 232, 232),
    (239, 222, 68),
    (178, 19, 24),
    (33, 87, 186),
    (18, 95, 32),
]

SPECTRA6_DEVICE_RGB = [
    (0, 0, 0),  # BLACK
    (255, 255, 255),  # WHITE
    (255, 255, 0),  # YELLOW
    (255, 0, 0),  # RED
    (0, 0, 255),  # BLUE
    (0, 255, 0),  # GREEN
]

SPECTRA6_DEVICE_INDEX_TO_RAW = [
    0,
    1,
    2,
    3,
    5,
    6,
]

# Check whether the input file exists
if not os.path.isfile(input_filename):
    print(f"Error: file {input_filename} does not exist")
    sys.exit(1)

# Read input image
input_image = Image.open(input_filename)

# Get the original image size
width, height = input_image.size

# Rotate 180 on request.
if rotate_180:
    input_image = input_image.rotate(180)

# Specified target size
if (width > height) != (target_width > target_height):
    print("Rotating image to match orientation.")
    input_image = input_image.rotate(angle, expand=True)
    width, height = input_image.size

if display_mode == "scale":
    # Computed scaling
    scale_ratio = max(target_width / width, target_height / height)

    # Calculate the size after scaling
    resized_width = int(width * scale_ratio)
    resized_height = int(height * scale_ratio)

    # Resize image
    output_image = input_image.resize((resized_width, resized_height))

    # Create the target image and center the resized image
    resized_image = Image.new("RGB", (target_width, target_height), (255, 255, 255))
    left = (target_width - resized_width) // 2
    top = (target_height - resized_height) // 2
    resized_image.paste(output_image, (left, top))
elif display_mode == "cut":
    # Calculate the fill size to add or the area to crop
    if width / height >= target_width / target_height:
        # The image aspect ratio is larger than the target aspect ratio, and padding needs to be added on the left and right
        delta_width = int(height * target_width / target_height - width)
        padding = (delta_width // 2, 0, delta_width - delta_width // 2, 0)
        box = (0, 0, width, height)
    else:
        # The image aspect ratio is smaller than the target aspect ratio and needs to be filled up and down
        delta_height = int(width * target_height / target_width - height)
        padding = (0, delta_height // 2, 0, delta_height - delta_height // 2)
        box = (0, 0, width, height)

    resized_image = ImageOps.pad(
        input_image.crop(box),
        size=(target_width, target_height),
        color=(255, 255, 255),
        centering=(0.5, 0.5),
    )


# Create a palette object
pal_image = Image.new("P", (1, 1))

palette = (
    tuple(v for rgb in SPECTRA6_REAL_WORD_RGB for v in rgb)
    + SPECTRA6_REAL_WORD_RGB[0] * 250
)
pal_image.putpalette(palette)

# The color quantization and dithering algorithms are performed, and the results are converted to RGB mode
quantized_image = resized_image.quantize(
    dither=display_dither, palette=pal_image
).convert("RGB")

# Save output image
basedir = os.path.dirname(input_filename)
basename = os.path.splitext(os.path.basename(input_filename))[0]
output_dir = os.path.join(basedir, "output")
os.makedirs(os.path.join(output_dir, "dithered"), exist_ok=True)
output_filename = os.path.join(output_dir, "dithered", basename + ".png")
quantized_image.save(output_filename)

# For each pixel, convert SPECTRA6 real-world RGB to device RGB
raw_bytes = bytearray()
raw_pending = 0
pixels = quantized_image.load()
for y in reversed(range(quantized_image.height)):
    for x in reversed(range(quantized_image.width)):
        r, g, b = pixels[x, y]
        index = SPECTRA6_REAL_WORD_RGB.index((r, g, b))
        pixels[x, y] = SPECTRA6_DEVICE_RGB[index]
        raw_value = SPECTRA6_DEVICE_INDEX_TO_RAW[index]
        assert raw_value < 8
        if (x & 1) == 0:
            raw_pending = raw_value
        else:
            raw_bytes.append((raw_pending << 4) | raw_value)
os.makedirs(os.path.join(output_dir, "device"), exist_ok=True)
output_filename = os.path.join(output_dir, "device", basename + ".bmp")
quantized_image.save(output_filename)

# Also, produce raw image suitable for SPECTRA6 use.
# Each byte has 2 pixels.
os.makedirs(os.path.join(output_dir, "raw"), exist_ok=True)
raw_output_filename = os.path.join(output_dir, "raw", basename + ".sp6")
with open(raw_output_filename, "wb") as f:
    f.write(raw_bytes)


print(
    f"Successfully converted {input_filename} to {output_filename} ({target_width} x {target_height})"
)
