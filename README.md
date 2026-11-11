Glitch Effects API

<p align="center">
  <img src="https://gor-mez8.onrender.com/render?text=GLITCH+EFFECTS&effect=glitch&font=monobold&w=700&h=120&size=72&c1=ff2e88&bg=transparent&fps=15&dur=3&seed=7" alt="Glitch Effects API" />
</p><p align="center">
  **A lightweight Go-powered animated text GIF generator.**
</p><p align="center">
  <a href="https://gor-mez8.onrender.com">Live Demo</a> ·
  <a href="https://gor-mez8.onrender.com/fonts">Fonts API</a> ·
  <a href="#api-documentation">API Documentation</a>
</p><p align="center">
  <img src="https://img.shields.io/badge/Go-1.20%2B-00ADD8?logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/Format-Animated%20GIF-ff2e88" alt="GIF" />
  <img src="https://img.shields.io/badge/Effects-9-blueviolet" alt="9 Effects" />
  <img src="https://img.shields.io/badge/Fonts-10-orange" alt="10 Fonts" />
  <img src="https://img.shields.io/badge/License-See%20Repository-lightgrey" alt="License" />
</p>---

Overview

Glitch Effects API is a lightweight HTTP service written in Go that generates animated text GIFs with digital distortion, retro VHS, RGB separation, noise, and other visual effects.

The service uses Go's image-processing libraries to render text, apply frame-by-frame effects, and encode the result into animated GIF format.

No JavaScript, browser-side rendering, external image-processing software, or SDK is required.

Simply construct a URL and receive a GIF.

Live Service

Resource| URL
Homepage| https://gor-mez8.onrender.com
GIF Generator| https://gor-mez8.onrender.com/render
Font List| https://gor-mez8.onrender.com/fonts

Features

- 9 animation effects for digital distortion and retro aesthetics.
- 10 built-in fonts embedded directly into the application.
- Custom TTF and OTF font support.
- Custom text, width, height, and font size.
- Custom HEX text and background colors.
- Transparent background support.
- Adjustable animation FPS and duration.
- Reproducible random effects using a fixed seed.
- GIF generation using Go's native image libraries.
- 256-entry GIF color palette with transparency.
- MD5-based file cache to avoid repeated rendering.
- HTTP caching headers.
- CORS support for cross-origin image requests.
- Direct integration with HTML, Markdown, and GitHub README.
- Lightweight deployment with Go.

Effect Gallery

The API provides nine built-in effects.

Effect| Description
"vhs"| Retro VHS distortion, scanlines, chromatic shifting, and noise
"static"| Random TV static noise
"dropout"| Horizontal signal interruptions and tracking distortions
"rgb"| RGB channel separation
"flicker"| Random brightness flickering
"ghost"| Motion trails and ghosting
"roll"| Vertical rolling distortion
"glitch"| Digital glitch, horizontal displacement, and noise
"type"| Animated character-by-character typing

Live Effect Previews

<p align="center">
  **VHS**
  <br/>
  <img src="https://gor-mez8.onrender.com/render?text=VHS+SIGNAL&effect=vhs&font=monobold&w=520&h=100&size=55&c1=00ffff&bg=0b0b0d&fps=15&dur=3&seed=7" alt="VHS effect"/>
</p><p align="center">
  **STATIC**
  <br/>
  <img src="https://gor-mez8.onrender.com/render?text=TV+STATIC&effect=static&font=monobold&w=520&h=100&size=55&c1=ffffff&bg=000000&fps=15&dur=3&seed=7" alt="Static effect"/>
</p><p align="center">
  **RGB**
  <br/>
  <img src="https://gor-mez8.onrender.com/render?text=RGB+SHIFT&effect=rgb&font=monobold&w=520&h=100&size=55&c1=00ffcc&bg=transparent&fps=15&dur=3&seed=7" alt="RGB effect"/>
</p><p align="center">
  **GLITCH**
  <br/>
  <img src="https://gor-mez8.onrender.com/render?text=SYSTEM+ERROR&effect=glitch&font=monobold&w=520&h=100&size=55&c1=ff2e88&bg=transparent&fps=15&dur=3&seed=7" alt="Glitch effect"/>
</p><p align="center">
  **GHOST**
  <br/>
  <img src="https://gor-mez8.onrender.com/render?text=GHOST+MODE&effect=ghost&font=monobold&w=520&h=100&size=55&c1=ffffff&bg=000000&fps=15&dur=3&seed=7" alt="Ghost effect"/>
</p><p align="center">
  **TYPE**
  <br/>
  <img src="https://gor-mez8.onrender.com/render?text=HELLO+WORLD&effect=type&font=monobold&w=520&h=100&size=55&c1=ff2e88&bg=transparent&fps=15&dur=3&seed=7" alt="Type effect"/>
</p>API Documentation

Base URL

https://gor-mez8.onrender.com

Endpoints

Method| Endpoint| Description| Response
GET| "/"| Web interface and documentation| HTML
GET| "/render"| Generate animated text GIF| "image/gif"
GET| "/fonts"| List available fonts| "text/plain"

1. Generate GIF

GET /render

Example:

https://gor-mez8.onrender.com/render?text=Nam2010&effect=glitch&font=monobold&w=520&h=100&size=70&c1=ff2e88&bg=transparent&fps=15&dur=3&seed=7

The endpoint returns an animated GIF directly.

Successful response:

HTTP/1.1 200 OK
Content-Type: image/gif
Cache-Control: public, max-age=86400
Content-Length: ...
Access-Control-Allow-Origin: *

If an unsupported effect is supplied, the API returns HTTP 400.

2. List Fonts

GET /fonts

Example:

curl https://gor-mez8.onrender.com/fonts

The response contains one font name per line.

3. Web Interface

GET /

The homepage includes:

- Interactive effect gallery.
- API documentation.
- Available font list.
- Parameter reference.
- Example generation URLs.

Query Parameters

Parameter| Default| Description
"text"| "content has been deleted"| Text to render
"effect"| "vhs"| Animation effect
"font"| "monobold"| Font identifier
"w"| "520"| GIF width in pixels
"h"| "60"| GIF height in pixels
"size"| "max(12, h-20)"| Font size in pixels
"c1"| "ff2e88"| Text color in HEX
"bg"| "transparent"| Background color
"fps"| "15"| Frames per second
"dur"| "2.5"| Animation duration in seconds
"seed"| Random| Random generator seed

Parameter Constraints

Parameter| Accepted behavior
"w"| Clamped between 16 and 800
"h"| Clamped between 16 and 400
"fps"| Clamped between 2 and 30
"dur"| Maximum 10 seconds
Frames| Between 1 and 120
"size"| Defaults to at least 12 px; supplied values are parsed as floating-point values
"seed"| Parsed as signed 64-bit integer

Frame count is calculated using:

n := int(float64(fps) * dur)
n = clampInt(n, 1, 120)

The GIF frame delay is calculated in centiseconds:

delay := 100 / fps

Color Parameters

Colors accept 3-digit or 6-digit hexadecimal values.

Examples:

c1=ff2e88
c1=00ffff
c1=ffffff
bg=000000
bg=transparent

The "#" prefix can also be parsed, but it should be URL-encoded as "%23" when included in a URL.

Invalid color values fall back to the default pink color.

Text Encoding

Use "+" or "%20" for spaces.

Example:

text=HELLO+WORLD

Special characters should be URL-encoded.

Available Fonts

The application embeds ten Go fonts from "golang.org/x/image/font/gofont".

Font| Style
"mono"| Monospaced
"monobold"| Monospaced Bold
"monoitalic"| Monospaced Italic
"monobolditalic"| Monospaced Bold Italic
"regular"| Regular
"bold"| Bold
"italic"| Italic
"bolditalic"| Bold Italic
"smallcaps"| Small Caps
"smallcapsitalic"| Small Caps Italic

Custom Fonts

Custom fonts are supported through ".ttf" and ".otf" files.

Default directory:

fonts/

Example:

fonts/
├── custom.ttf
├── cyberpunk.otf
└── pixel.ttf

Use the filename without the extension:

font=cyberpunk

Font naming rules:

- Lowercase letters.
- Numbers.
- Underscores.
- Hyphens.
- Maximum 40 characters.

Unknown or invalid fonts automatically fall back to "monobold".

Usage Examples

Basic Glitch

https://gor-mez8.onrender.com/render?text=HELLO&effect=glitch&seed=7

Neon Pink

https://gor-mez8.onrender.com/render?text=NAM2010&effect=glitch&font=monobold&w=600&h=120&size=80&c1=ff2e88&bg=transparent&fps=15&dur=3&seed=7

Retro VHS

https://gor-mez8.onrender.com/render?text=RETRO&effect=vhs&font=bold&w=600&h=120&size=80&c1=00ffff&bg=000000&fps=20&dur=3&seed=7

RGB Distortion

https://gor-mez8.onrender.com/render?text=CYBER&effect=rgb&font=monobold&w=600&h=120&size=80&c1=00ffcc&bg=transparent&fps=15&dur=3&seed=7

Typing Animation

https://gor-mez8.onrender.com/render?text=HELLO+WORLD&effect=type&font=mono&w=600&h=120&size=60&c1=ffffff&bg=000000&fps=15&dur=3&seed=7

Dark Terminal Style

https://gor-mez8.onrender.com/render?text=SYSTEM+ONLINE&effect=static&font=mono&w=700&h=100&size=65&c1=ffffff&bg=0b0b0d&fps=15&dur=3&seed=7

Integration

HTML

<img
  src="https://gor-mez8.onrender.com/render?text=HELLO&effect=glitch&seed=7"
  alt="Glitch Text"
/>

Markdown

![Glitch Text](https://gor-mez8.onrender.com/render?text=HELLO&effect=glitch&seed=7)

Centered GitHub Banner

<p align="center">
  <img
    src="https://gor-mez8.onrender.com/render?text=MY+PROJECT&effect=glitch&font=monobold&w=700&h=120&size=75&c1=ff2e88&bg=transparent&fps=15&dur=3&seed=7"
    alt="My Project"
  />
</p>

Python

import requests

url = "https://gor-mez8.onrender.com/render"

params = {
    "text": "Nam2010",
    "effect": "glitch",
    "font": "monobold",
    "w": 520,
    "h": 100,
    "size": 70,
    "c1": "ff2e88",
    "bg": "transparent",
    "fps": 15,
    "dur": 3,
    "seed": 7,
}

response = requests.get(url, params=params, timeout=60)
response.raise_for_status()

with open("glitch.gif", "wb") as f:
    f.write(response.content)

print("GIF saved successfully!")

cURL

curl -L \
  "https://gor-mez8.onrender.com/render?text=Nam2010&effect=glitch&seed=7" \
  -o glitch.gif

How It Works

The rendering pipeline is implemented entirely in Go.

flowchart TD
    A["HTTP Request"] --> B["Parse Query Parameters"]
    B --> C["Resolve Font"]
    C --> D["Generate MD5 Cache Key"]
    D --> E{"Cached GIF Exists?"}
    E -->|Yes| F["Read Cached GIF"]
    E -->|No| G["Render Base RGBA Image"]
    G --> H["Apply Animation Effect"]
    H --> I["Generate Animation Frames"]
    I --> J["Convert to Paletted Images"]
    J --> K["Encode Animated GIF"]
    K --> L["Save GIF Cache"]
    F --> M["Return image/gif"]
    L --> M

Rendering Pipeline

1. Parse incoming HTTP query parameters.
2. Resolve the requested font.
3. Calculate the frame count and random seed.
4. Generate a cache key from rendering parameters.
5. Check whether the GIF already exists in the cache.
6. Render the base text into an RGBA image.
7. Apply the selected animation effect to each frame.
8. Convert RGBA frames into paletted images.
9. Encode all frames into an animated GIF.
10. Save the generated GIF and return it to the client.

Image Processing

The application uses:

- "image.RGBA" for intermediate rendering.
- Alpha compositing for transparency and overlays.
- RGB channel shifting for chromatic effects.
- Random noise generation for distortion.
- Horizontal band displacement for signal interruptions.
- Scanlines for retro display effects.
- "image.Paletted" for GIF conversion.
- "image/gif" for animated GIF encoding.

GIF Palette

The encoder constructs a 256-entry color palette.

- Palette index 0 is reserved for transparency.
- The remaining entries are generated from a six-level RGB channel mapping.
- Pixels with alpha below 32 are converted to transparent pixels.

GIF is an indexed-color format, so output is not a lossless representation of the original RGBA frames.

Caching System

The application uses file-based caching.

Cache Key

The cache key is calculated using MD5:

h := md5.Sum([]byte(s))

The key incorporates:

- Text content.
- Effect.
- Resolved font.
- Width and height.
- Text and background colors.
- Font size.
- FPS.
- Frame count.
- Seed.

Cache Directory

Default:

cache/

Cached files are stored as:

cache/
├── <md5>.gif
├── <md5>.gif
└── ...

Seed Behavior

Without a "seed" parameter, the application generates a random seed for each request.

With a fixed seed:

seed=7

The random generator starts from the same seed, allowing the same rendering parameters to reproduce the same animation.

This also allows the cache to reuse the same GIF file.

Recommendation: Always specify a fixed seed when embedding GIFs into GitHub README files or websites.

HTTP Cache

Successful GIF responses include:

Cache-Control: public, max-age=86400
Content-Type: image/gif
Access-Control-Allow-Origin: *

The HTTP cache lifetime is 24 hours.

Installation

Requirements

- Go 1.20 or newer.
- Git.
- Internet access for downloading Go dependencies.

Clone Repository

git clone https://github.com/USERNAME/REPOSITORY.git
cd REPOSITORY

Replace the repository URL with the actual project repository.

Install Dependencies

go mod tidy

The application uses:

golang.org/x/image

Run

go run .

Default address:

http://localhost:8080

Build

go build -trimpath -ldflags="-s -w" -o glitch-api .

Run:

./glitch-api

Environment Variables

The application supports three environment variables.

Variable| Default| Description
"PORT"| "8080"| HTTP server port
"CACHE_DIR"| "cache"| GIF cache directory
"FONTS_DIR"| "fonts"| Custom font directory

Example

export PORT=8080
export CACHE_DIR=cache
export FONTS_DIR=fonts

go run .

One-Line Configuration

PORT=8080 CACHE_DIR=cache FONTS_DIR=fonts go run .

The application automatically creates the cache and font directories when starting.

Deployment on Render

The application can be deployed as a Go Web Service.

Build Command

go build -trimpath -ldflags="-s -w" -o app .

Start Command

./app

Environment Variables

PORT=10000
CACHE_DIR=cache
FONTS_DIR=fonts

Render provides the service port through its environment. The application listens on ":$PORT".

Persistent Storage

The GIF cache is stored on the local filesystem.

If the deployment environment uses an ephemeral filesystem, cached files may disappear when the instance restarts or is redeployed.

For persistent cache storage, configure a persistent disk and point "CACHE_DIR" to its mounted path.

Custom fonts should also be included in the deployment or stored on persistent storage if they need to survive deployments.

Performance Considerations

- Fixed seeds improve cache reuse.
- Repeated requests with identical parameters avoid unnecessary rendering.
- Lower FPS reduces the number of frames.
- Smaller dimensions reduce image-processing work.
- Shorter animation duration reduces frame count.
- The first uncached request requires actual GIF generation.

The maximum image dimensions are 800 × 400 pixels, with a maximum of 120 frames per animation.

Error Handling

Condition| Behavior
Missing text| Uses "content has been deleted"
Missing effect| Uses "vhs"
Unknown effect| HTTP 400
Invalid font| Falls back to "monobold"
Invalid width| Uses default or clamps to allowed range
Invalid height| Uses default or clamps to allowed range
Invalid FPS| Uses default or clamps to allowed range
GIF encoding failure| HTTP 500

Project Structure

The core application is implemented in "main.go".

A typical deployment layout:

project/
├── main.go
├── go.mod
├── go.sum
├── fonts/
│   ├── custom.ttf
│   └── pixel.otf
└── cache/
    └── generated GIF files

"go.mod" and "go.sum" manage Go dependencies.

The "fonts/" directory is optional because all ten built-in fonts are embedded in the application.

The "cache/" directory is created automatically.

Security and Usage Notes

- Do not expose sensitive information through generated text.
- The API currently provides unrestricted cross-origin image access.
- Avoid sending excessively large volumes of generation requests.
- Use a fixed seed to reduce unnecessary cache creation.
- The MD5 cache key is used for cache identification, not cryptographic security.
- The public deployment may experience cold starts depending on hosting conditions.

Technical Stack

Component| Technology
Programming language| Go
HTTP server| "net/http"
Image processing| Go "image" package
Intermediate image format| RGBA
Font rendering| "golang.org/x/image/font"
Built-in fonts| GoFont
Custom fonts| OpenType / TrueType
GIF encoding| "image/gif"
Random generation| "math/rand"
Cache hashing| MD5
Cache storage| Local filesystem
Hosting| Render

License

See the repository's LICENSE file for licensing terms.

If no LICENSE file is provided, all rights remain with the respective copyright holder.

---

Contributing

Contributions, bug reports, and feature suggestions are welcome.

Potential improvements include:

- Additional glitch effects.
- More built-in fonts.
- Better GIF color quantization.
- Improved rendering performance.
- Cache management and expiration.
- More output formats.
- Additional API options.

Please open an issue or submit a pull request through the project repository.

---

<p align="center">
  **Glitch Effects API**
</p><p align="center">
  Generate. Distort. Animate.
</p><p align="center">
  Built with Go ❤️
</p>
