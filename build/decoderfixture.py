# SPDX-License-Identifier: AGPL-3.0-only
"""Write test/ui/decoders.pdf: one page per picture encoding pdf.js decodes through a separate module
(/pending 984). Run it only to change the fixture; the PDF is committed so tier 3 needs no Pillow.

    python3 -I build/decoderfixture.py test/ui/decoders.pdf

Origin: adapted from RelaxTax's tools/pagefixtures/make.py (Dan's, AGPL-3.0-only; its pending 98 and 100),
which drew the same synthetic scan for the same defect. The stencil-mask page is nib's own, shaped after a
Canon iR-ADV copier scan (mixed raster content: a JPEG background, every word a 1-bit CCITT /ImageMask).

Every page draws its picture in the same rectangle (56, 436, 500 x 300 pt on US Letter):
  1. baseline JPEG, the control: pdf.js decodes it with no module, so it draws whatever wasmUrl says;
  2. JBIG2 (an embedded MMR generic region whose bytes are libtiff's Group 4 strip);
  3. CCITT Group 4 (K -1), the same strip;
  4. JPEG 2000 (Pillow's OpenJPEG, lossless);
  5. a pale JPEG background with the CCITT strip over it as an /ImageMask stencil painted in black.
Nothing here is a real document: the picture is a box grid and bars.

Needs Pillow built with libtiff and OpenJPEG. The bytes depend on those libraries' versions.
"""

import io
import struct
import sys

from PIL import Image, ImageDraw

W, H = 612, 792
PW, PH = 500, 300
X, Y = 56, H - 56 - PH


def stream(d, s):
    return b"<< " + d + b" /Length %d >>\nstream\n" % len(s) + s + b"\nendstream"


def pdf(pages):
    """pages: (content bytes, {name: (dict bytes, stream bytes)})."""
    objs = [None, None]
    kids = []

    def add(body):
        objs.append(body)
        return len(objs)

    for content, images in pages:
        xo = b" ".join(b"/%s %d 0 R" % (n.encode(), add(stream(*img))) for n, img in images.items())
        c = add(stream(b"", content))
        kids.append(add(b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Resources << /XObject << %s >> >> /Contents %d 0 R >>"
                        % (W, H, xo, c)))
    objs[0] = b"<< /Type /Catalog /Pages 2 0 R >>"
    objs[1] = b"<< /Type /Pages /Kids [%s] /Count %d >>" % (b" ".join(b"%d 0 R" % k for k in kids), len(kids))
    out = io.BytesIO()
    out.write(b"%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
    offs = []
    for i, o in enumerate(objs, 1):
        offs.append(out.tell())
        out.write(b"%d 0 obj\n" % i + o + b"\nendobj\n")
    x = out.tell()
    out.write(b"xref\n0 %d\n0000000000 65535 f \n" % (len(objs) + 1))
    for o in offs:
        out.write(b"%010d 00000 n \n" % o)
    out.write(b"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n" % (len(objs) + 1, x))
    return out.getvalue()


def scan(mode):
    """A synthetic scan: a box grid and bars."""
    im = Image.new(mode, (PW, PH), 255 if mode in ("L", "1") else (255, 255, 255))
    d = ImageDraw.Draw(im)
    ink = 0 if mode in ("L", "1") else (20, 20, 60)
    for i in range(0, PW, PW // 6):
        d.rectangle([i + 4, 8, i + PW // 6 - 4, PH // 3], outline=ink, width=3)
    for j in range(PH // 2, PH - 10, 14):
        d.rectangle([10, j, PW - 10 - (j * 7) % (PW // 2), j + 6], fill=ink)
    return im


def g4(im):
    """im's ITU-T T.6 bytes, libtiff's one strip, ink coded as black runs."""
    b = io.BytesIO()
    im.point(lambda p: 255 - p).save(b, "TIFF", compression="group4")
    t = Image.open(io.BytesIO(b.getvalue()))
    off, n = t.tag_v2[273][0], t.tag_v2[279][0]
    return b.getvalue()[off:off + n]


def jbig2(im):
    """An embedded JBIG2 stream: a page information segment and an immediate generic region coded MMR."""
    def seg(num, kind, data):
        return struct.pack(">IBBBI", num, kind, 0, 1, len(data)) + data

    page = struct.pack(">IIIIBH", PW, PH, 0, 0, 0, 0)
    region = struct.pack(">IIIIB", PW, PH, 0, 0, 0) + bytes([1]) + g4(im)
    return seg(0, 48, page) + seg(1, 38, region) + seg(2, 49, b"")


def jpeg(im):
    b = io.BytesIO()
    im.save(b, "JPEG", quality=80)
    return b.getvalue()


def main(out):
    g = scan("1")
    place = b"%d 0 0 %d %d %d cm" % (PW, PH, X, Y)
    image = b"/Type /XObject /Subtype /Image /Width %d /Height %d " % (PW, PH)
    gray1 = image + b"/ColorSpace /DeviceGray /BitsPerComponent 1 "
    ccitt = b"/Filter /CCITTFaxDecode /DecodeParms << /K -1 /Columns %d /Rows %d >>" % (PW, PH)
    j2 = io.BytesIO()
    scan("RGB").save(j2, "JPEG2000", irreversible=False)
    pale = Image.new("L", (PW, PH), 228)

    def one(name, d, s):
        return b"q " + place + b" /%s Do Q" % name, {name.decode(): (d, s)}

    pages = [
        one(b"Dc", image + b"/ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /DCTDecode", jpeg(scan("L"))),
        one(b"Jb", gray1 + b"/Filter /JBIG2Decode", jbig2(g)),
        one(b"Cf", gray1 + ccitt, g4(g)),
        one(b"Jx", image + b"/Filter /JPXDecode", j2.getvalue()),
        (b"q " + place + b" /Bg Do Q q 0 g " + place + b" /Mk Do Q",
         {"Bg": (image + b"/ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /DCTDecode", jpeg(pale)),
          "Mk": (image + b"/ImageMask true /BitsPerComponent 1 " + ccitt, g4(g))}),
    ]
    with open(out, "wb") as f:
        f.write(pdf(pages))


if __name__ == "__main__":
    main(sys.argv[1])
