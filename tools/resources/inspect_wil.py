#!/usr/bin/env python3
"""严格探测旧版未压缩256色WIL/WIX，保留索引并导出少量研发样例。"""

from __future__ import annotations

import argparse
import hashlib
import json
import struct
import sys
from pathlib import Path


PROJECT = Path(__file__).resolve().parents[2]
SAMPLE_ROOT = PROJECT / "client/assets/imported/samples"
SOURCE_ROOTS = {
    "client_180_named": PROJECT.parent / "传奇1.80客户端/Data",
    "client_resources": PROJECT.parent / "客户端资源/Data",
}
DEFAULT_PACKAGES = ("Hum", "Weapon", "Hair", "Mon1", "Tiles", "Magic")
MAX_FRAME_COUNT = 1_000_000
MAX_DIMENSION = 8192
MAX_FRAME_PIXELS = 16_777_216


class UnsupportedFormat(ValueError):
    """没有明确格式证据时停止，不输出猜测图像。"""


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def readonly_path(path: Path, root: Path) -> Path:
    if root not in path.parents:
        raise ValueError("文件不在固定来源范围")
    cursor = path
    while True:
        if cursor.is_symlink():
            raise ValueError("拒绝符号链接")
        if cursor == root:
            break
        cursor = cursor.parent
    return path


def locate_pair(source_id: str, package: str) -> tuple[Path, Path]:
    if not package or Path(package).name != package or any(c in package for c in ("/", "\\", ".")):
        raise ValueError("包名只接受不含路径和扩展名的文件名")
    root = SOURCE_ROOTS[source_id]
    if root.is_symlink() or not root.is_dir():
        raise ValueError("固定来源目录不可用")
    matches = [p for p in root.iterdir() if p.stem.casefold() == package.casefold()]
    wil = [p for p in matches if p.suffix.casefold() == ".wil"]
    wix = [p for p in matches if p.suffix.casefold() == ".wix"]
    if len(wil) != 1 or len(wix) != 1:
        alternatives = sorted({p.suffix.casefold() for p in matches})
        raise UnsupportedFormat(f"需要唯一WIL/WIX配对；当前扩展名为{alternatives}，WZL/WZX等不在支持范围")
    return readonly_path(wil[0], root), readonly_path(wix[0], root)


def probe(wil: Path, wix: Path) -> dict:
    with wil.open("rb") as stream:
        header = stream.read(60)
        if len(header) != 60 or not header.startswith(b"#ILIB v1.0-WEMADE Entertainment inc."):
            raise UnsupportedFormat("未识别的WIL签名；只支持内嵌256色板的v1.0未压缩格式")
        declared_count, palette_count, palette_bytes, version = struct.unpack_from("<IIII", header, 44)
        if header[40] in (1, 6) or header[48] == 32 or version != 0 or palette_count != 256 or palette_bytes != 1024:
            raise UnsupportedFormat("压缩/16位/32位/非零版本或未知色板布局不支持")
        palette_raw = stream.read(255 * 4)
        if len(palette_raw) != 1020:
            raise UnsupportedFormat("内嵌色板截断")
        colors = struct.unpack("<255I", palette_raw)
        if any(color & 0xFF000000 for color in colors):
            raise UnsupportedFormat("色板存在未知高字节语义，不猜测透明度")
        # 对应Crystal：new int[256]的第0项保留0；从第1项读取并加FF透明度。
        palette = [(0, 0, 0, 0)] + [((c >> 16) & 255, (c >> 8) & 255, c & 255, 255) for c in colors]
    index_bytes = wix.read_bytes()
    if len(index_bytes) < 48 or not index_bytes.startswith(b"#INDX v1.0-WEMADE Entertainment inc."):
        raise UnsupportedFormat("WIX签名或长度不支持")
    if (len(index_bytes) - 48) % 4:
        raise UnsupportedFormat("WIX索引长度不是4字节整数的完整序列")
    frame_count = (len(index_bytes) - 48) // 4
    wix_count = struct.unpack_from("<I", index_bytes, 44)[0]
    if not 0 < frame_count <= MAX_FRAME_COUNT or declared_count != frame_count or wix_count != frame_count:
        raise UnsupportedFormat("WIL/WIX声明帧数与索引实际长度不符或超过工具限额")
    offsets = list(struct.unpack_from(f"<{frame_count}I", index_bytes, 48))
    file_size = wil.stat().st_size
    distinct = sorted(set(offsets) - {0})
    next_offset = dict(zip(distinct, distinct[1:] + [file_size]))
    frames = []
    with wil.open("rb") as stream:
        for frame_index, offset in enumerate(offsets):
            if offset == 0:
                frames.append({"frame_index": frame_index, "source_offset": 0, "width": 0, "height": 0,
                               "offset_x": None, "offset_y": None, "pixel_bytes": 0,
                               "declared_empty": True, "empty_reason": "zero_index", "png_path": None})
                continue
            if offset < 1080 or offset + 8 > file_size:
                raise UnsupportedFormat(f"帧{frame_index}索引越界")
            stream.seek(offset)
            width, height, offset_x, offset_y = struct.unpack("<hhhh", stream.read(8))
            if width < 0 or height < 0 or width > MAX_DIMENSION or height > MAX_DIMENSION or width * height > MAX_FRAME_PIXELS:
                raise UnsupportedFormat(f"帧{frame_index}尺寸不合法或超过限额")
            pixel_bytes = width * height
            if offset + 8 + pixel_bytes > next_offset[offset]:
                raise UnsupportedFormat(f"帧{frame_index}像素数据超出下一已知帧或文件边界")
            frames.append({"frame_index": frame_index, "source_offset": offset, "width": width, "height": height,
                           "offset_x": offset_x, "offset_y": offset_y, "pixel_bytes": pixel_bytes,
                           "declared_empty": not bool(pixel_bytes), "empty_reason": "zero_dimension" if not pixel_bytes else None,
                           "small_frame_pixels_le_4": pixel_bytes <= 4, "png_path": None})
    return {"format_id": "wil_v1_palette256_uncompressed_version0", "frame_count": frame_count,
            "palette": palette, "palette_sha256": hashlib.sha256(palette_raw).hexdigest(), "frames": frames,
            "header": {"wil_header_bytes": 60, "wix_header_bytes": 48, "frame_header_bytes": 8,
                       "palette_count": palette_count, "palette_storage_bytes": 1020, "palette_declared_bytes": palette_bytes,
                       "version": version, "pixel_storage": "8bit_palette_index", "row_order": "bottom_up",
                       "row_padding": 0, "frame_count_declared": declared_count}}


def decode_frame(wil: Path, frame: dict, palette: list) -> bytes:
    if frame["declared_empty"]:
        return b""
    width, height = frame["width"], frame["height"]
    with wil.open("rb") as stream:
        stream.seek(frame["source_offset"] + 8)
        pixels = stream.read(frame["pixel_bytes"])
    if len(pixels) != frame["pixel_bytes"]:
        raise UnsupportedFormat("像素数据截断")
    palette_rgba = [bytes(color) for color in palette]
    rows = [b"".join(palette_rgba[value] for value in pixels[start:start + width])
            for start in range(0, width * height, width)]
    return b"".join(reversed(rows))


def code_evidence() -> dict:
    path = PROJECT / "资料参考/crystal/LibraryEditor/Graphics/WeMadeLibrary.cs"
    text = path.read_text(encoding="utf-8-sig")
    markers = ("_fStream.Seek(48, SeekOrigin.Begin)", "for (int i = 1; i < _palette.Length; i++)",
               "reader.ReadBytes(_version == 0 ? 48 : 52)", "Width = reader.ReadInt16()",
               "case 0://wemade wil file uncompressed", "for (int y = Height - 1; y >= 0; y--)")
    references = []
    for marker in markers:
        found = [no for no, line in enumerate(text.splitlines(), 1) if marker in line]
        if not found:
            raise ValueError("本地Crystal格式证据缺少预期标记，请人工复核工具后再用")
        references.append({"marker": marker, "lines": found})
    return {"path": path.relative_to(PROJECT).as_posix(), "sha256": sha256_file(path), "references": references,
            "is_official_180_proof": False}


def validate_output_path(output: Path) -> None:
    if not output.is_absolute():
        output = PROJECT / output
    resolved = output.resolve()
    if resolved != SAMPLE_ROOT and SAMPLE_ROOT not in resolved.parents:
        raise ValueError("样例输出必须位于当前项目client/assets/imported/samples/内")
    cursor = output
    while cursor != PROJECT:
        if cursor.is_symlink():
            raise ValueError("样例输出不接受符号链接")
        cursor = cursor.parent


def convert(source_id: str, package: str, args, evidence: dict) -> dict:
    wil, wix = locate_pair(source_id, package)
    data = probe(wil, wix)
    wil_sha, wix_sha = sha256_file(wil), sha256_file(wix)
    summary = {"source_id": source_id, "package": wil.stem, "format": data["format_id"],
               "frame_count": data["frame_count"], "declared_empty_frames": sum(f["declared_empty"] for f in data["frames"]),
               "small_frames_pixels_le_4": sum(f.get("small_frame_pixels_le_4", False) for f in data["frames"]),
               "status": "unverified", "frames_head": data["frames"][:3]}
    if args.probe_only:
        return summary
    try:
        from PIL import Image, __version__ as pillow_version
    except ImportError as error:
        raise ValueError("PNG导出需要已锁定Pillow；请用用户级minimir工具链的tools-venv/bin/python3执行，或先使用--probe-only；依赖见conf/toolchain/python.lock.json") from error
    output = Path(args.output)
    if not output.is_absolute():
        output = PROJECT / output
    output = output / source_id / wil.stem
    validate_output_path(output)
    requested = [int(value) for value in args.frames.split(",")] if args.frames else None
    if requested is not None:
        if len(requested) > 16 or len(set(requested)) != len(requested) or any(i < 0 or i >= data["frame_count"] for i in requested):
            raise ValueError("明确帧号须唯一、合法且不超过16项")
        selected = [data["frames"][i] for i in requested]
    else:
        selected = []
        for frame in data["frames"]:
            if frame["declared_empty"] or frame["pixel_bytes"] < 16:
                continue
            rgba = decode_frame(wil, frame, data["palette"])
            if not any(rgba[3::4]):
                continue
            selected.append(frame)
            if len(selected) >= args.sample_count:
                break
    if not args.check:
        output.mkdir(parents=True, exist_ok=True)
    exports = []
    for frame in selected:
        number = frame["frame_index"]
        if frame["declared_empty"]:
            exports.append({"frame_index": number, "status": "unverified", "png_path": None,
                            "reason": "原始空帧仅保留元数据，不伪造1×1替代图"})
            continue
        rgba = decode_frame(wil, frame, data["palette"])
        png = output / f"frame_{number:06d}.png"
        image = Image.frombytes("RGBA", (frame["width"], frame["height"]), rgba)
        if args.check:
            if not png.is_file():
                raise ValueError(f"缺少样例PNG：{png.relative_to(PROJECT)}")
        else:
            if png.exists():
                with Image.open(png) as existing:
                    if existing.mode != "RGBA" or existing.size != image.size or existing.tobytes() != rgba:
                        raise ValueError(f"已存在样例PNG内容冲突，不覆盖：{png.relative_to(PROJECT)}")
            else:
                image.save(png, format="PNG")
        with Image.open(png) as reopened:
            if reopened.mode != "RGBA" or reopened.size != image.size or reopened.tobytes() != rgba:
                raise ValueError("PNG回读与源像素解码不一致")
        frame["png_path"] = png.relative_to(PROJECT).as_posix()
        exports.append({"frame_index": number, "status": "unverified", "png_path": frame["png_path"],
                        "png_sha256": sha256_file(png), "decoded_rgba_sha256": hashlib.sha256(rgba).hexdigest(),
                        "png_roundtrip_verified": True, "width": frame["width"], "height": frame["height"],
                        "offset_x": frame["offset_x"], "offset_y": frame["offset_y"],
                        "alpha_nonzero_pixels": sum(alpha != 0 for alpha in rgba[3::4])})
    if sha256_file(wil) != wil_sha or sha256_file(wix) != wix_sha:
        raise ValueError("转换期间来源发生变化；样例不能作为通过结果")
    manifest = {
        "schema_version": 1, "status": "unverified", "runtime_ready": False,
        "purpose": "有限研发样例，格式与PNG回读检查不等于完整美术或方向/技能映射验收",
        "source_id": source_id, "wil_path": str(wil), "wix_path": str(wix), "wil_sha256": wil_sha, "wix_sha256": wix_sha,
        "format": data["format_id"], "format_header": data["header"], "palette_sha256": data["palette_sha256"],
        "transparent_index": 0, "other_palette_entries_alpha": 255,
        "palette_rule_evidence": "Crystal new int[256]第0项为0，从第1项读取BGR整数并加FF alpha；无外置/猜测色板",
        "code_evidence": evidence, "tool_sha256": sha256_file(Path(__file__)), "pillow_version": pillow_version,
        "frame_count": data["frame_count"], "sample_selection": "explicit_indices" if requested is not None else "first_visible_frames_with_at_least_16_pixels",
        "sample_count_requested": len(requested) if requested is not None else args.sample_count,
        "exported_png_count": sum(x["png_path"] is not None for x in exports),
        "direction_stride": None, "action_mapping": None, "timing_ms": None,
        "crop_or_recenter": False, "pixel_resample": False,
        "empty_frames": "保留全部原索引和尺寸；零索引/零尺寸仅记录元数据；4像素等小帧不自动当空帧删除",
        "frames": data["frames"], "samples": exports,
    }
    expected = json.dumps(manifest, ensure_ascii=False, indent=2) + "\n"
    path = output / "frames.json"
    if args.check:
        if not path.is_file() or path.read_text(encoding="utf-8") != expected:
            raise ValueError(f"样例元数据缺失或过期：{path.relative_to(PROJECT)}")
    else:
        path.write_text(expected, encoding="utf-8")
    summary.update({"exported_png_count": manifest["exported_png_count"], "metadata": path.relative_to(PROJECT).as_posix(),
                    "png_roundtrip_verified": True})
    return summary


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-id", choices=tuple(SOURCE_ROOTS), default="client_180_named")
    parser.add_argument("--package", nargs="+", default=list(DEFAULT_PACKAGES))
    parser.add_argument("--sample-count", type=int, default=2, help="每包自动选择1～8个非空样例")
    parser.add_argument("--frames", help="单包明确指定原始帧号，例如0,1,40；不重排编号")
    parser.add_argument("--output", default="client/assets/imported/samples")
    parser.add_argument("--probe-only", action="store_true", help="仅探测/打印概要，不需要Pillow，不写文件")
    parser.add_argument("--check", action="store_true", help="按原包重新解码并回读已有PNG/元数据，不改文件")
    args = parser.parse_args()
    if not 1 <= args.sample_count <= 8 or (args.frames and len(args.package) != 1) or (args.check and args.probe_only):
        parser.error("样例数量须为1～8；--frames仅适用单包；--check与--probe-only不能同时使用")
    evidence = code_evidence()
    results, failures = [], []
    for package in args.package:
        try:
            results.append(convert(args.source_id, package, args, evidence))
        except (OSError, ValueError, struct.error) as error:
            failures.append({"package": package, "status": "unsupported_or_failed", "reason": str(error), "guessed_png_created": False})
    print(json.dumps({"status": "checked" if args.check and not failures else ("samples_unverified" if not failures else "unsupported_or_failed"),
                      "runtime_ready": False, "results": results, "failures": failures}, ensure_ascii=False))
    return 2 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
