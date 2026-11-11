#!/usr/bin/env python3
"""画 muse2api Studio 的 exe 图标：与网页端同款的靛蓝→紫渐变圆角方块 + 白色 ✦ 星芒。
输出：icon.png（512）与 muse2api-studio.ico（256/128/64/48/32/16）。
"""
from PIL import Image, ImageDraw, ImageFilter

S = 2048                      # 超采样画布
C = S // 2                    # 中心
TOP = (99, 102, 241)          # #6366f1 靛蓝
BOT = (168, 85, 247)          # #a855f7 紫
WHITE = (255, 255, 255, 255)


def lerp(a, b, t):
    return tuple(round(a[i] + (b[i] - a[i]) * t) for i in range(3))


def make_gradient(size):
    """对角渐变（左上靛蓝 → 右下紫），512 见方再放大，省算力。"""
    g = Image.new("RGB", (size, size))
    px = g.load()
    for y in range(size):
        for x in range(size):
            t = (x + y) / (2 * (size - 1))
            px[x, y] = lerp(TOP, BOT, t)
    return g


def rounded_mask(size, radius):
    m = Image.new("L", (size, size), 0)
    d = ImageDraw.Draw(m)
    d.rounded_rectangle([0, 0, size - 1, size - 1], radius=radius, fill=255)
    return m


def quad(p0, p1, p2, n=24):
    """二次贝塞尔采样点（不含终点，调用方收尾）。"""
    pts = []
    for i in range(n):
        t = i / n
        x = (1 - t) ** 2 * p0[0] + 2 * (1 - t) * t * p1[0] + t ** 2 * p2[0]
        y = (1 - t) ** 2 * p0[1] + 2 * (1 - t) * t * p1[1] + t ** 2 * p2[1]
        pts.append((x, y))
    return pts


def sparkle_points(cx, cy, r_outer, r_inner, bow=0.32):
    """四芒星轮廓：4 个尖角 + 4 个内凹点，边向中心弯（bow 越大越凹）。"""
    import math
    tips = [(cx, cy - r_outer), (cx + r_outer, cy),
            (cx, cy + r_outer), (cx - r_outer, cy)]
    inners = []
    for k in range(4):
        # 内凹点按顺时针排：NE、SE、SW、NW，与尖角 N、E、S、W 的顺序对齐；
        # 角度若从 45° 起，配对会错一位，星芒会被剪切成风车状。
        ang = math.radians(-45 + 90 * k)
        inners.append((cx + r_inner * math.cos(ang), cy + r_inner * math.sin(ang)))
    pts = []
    for k in range(4):
        t = tips[k]
        i_prev = inners[(k - 1) % 4]
        i_next = inners[k]
        # 边：内凹点 -> 尖角，控制点向中心拉，边呈内弯弧线
        for p0, p2 in ((i_prev, t), (t, i_next)):
            mx, my = (p0[0] + p2[0]) / 2, (p0[1] + p2[1]) / 2
            ctrl = (mx + (cx - mx) * bow, my + (cy - my) * bow)
            pts += quad(p0, ctrl, p2)
    return pts


def main():
    # 底：对角渐变圆角方块
    grad = make_gradient(512).resize((S, S), Image.LANCZOS)
    base = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    base.paste(grad, (0, 0))
    base.putalpha(rounded_mask(S, 448))

    # 左上柔光
    glow = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    gd = ImageDraw.Draw(glow)
    gd.ellipse([C - 1150, -750, C + 350, 750], fill=(255, 255, 255, 46))
    glow = glow.filter(ImageFilter.GaussianBlur(180))
    base = Image.alpha_composite(base, glow)

    # 主星芒：先画一层模糊白光打底，再画实星
    halo = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    ImageDraw.Draw(halo).polygon(sparkle_points(C, C, 660, 140, bow=0.10), fill=(255, 255, 255, 110))
    halo = halo.filter(ImageFilter.GaussianBlur(70))
    base = Image.alpha_composite(base, halo)

    star = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    ImageDraw.Draw(star).polygon(sparkle_points(C, C, 640, 130, bow=0.10), fill=WHITE)
    base = Image.alpha_composite(base, star)

    # 右下小星点缀
    small = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    ImageDraw.Draw(small).polygon(
        sparkle_points(int(C + 470), int(C + 470), 150, 34, bow=0.10), fill=(255, 255, 255, 235))
    base = Image.alpha_composite(base, small)

    icon = base.resize((512, 512), Image.LANCZOS)
    icon.save("/home/hatch/workspace/muse-studio/icon.png")
    icon.save("/home/hatch/workspace/muse-studio/muse2api-studio.ico",
              sizes=[(256, 256), (128, 128), (64, 64), (48, 48), (32, 32), (16, 16)])
    print("icon.png + muse2api-studio.ico done")


if __name__ == "__main__":
    main()
