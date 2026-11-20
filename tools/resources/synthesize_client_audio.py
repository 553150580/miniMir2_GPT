"""生成试玩的原创循环音乐与短音效。只写独立的客户端音频资源目录。"""

from array import array
from math import cos, exp, pi, sin, tanh
from pathlib import Path
from random import Random
import sys
import wave


RATE = 24000
OUTPUT = Path(__file__).resolve().parents[2] / 'client/assets/resources/audio'
TAU = 2 * pi


def frequency(note):
    return 440 * 2 ** ((note - 69) / 12)


def edge(t, length, attack, release):
    return min(1, t / attack, (length - t) / release)


def add(samples, start, length, voice, wrap=False):
    offset = round(start * RATE)
    frames = min(round(length * RATE), len(samples))
    for i in range(frames):
        index = offset + i
        if index >= len(samples) and not wrap:
            break
        samples[index % len(samples)] += voice(i / RATE)


def pluck(note, length, gain):
    f = TAU * frequency(note)

    def voice(t):
        texture = sin(f * t) + 0.28 * exp(-4 * t) * sin(2 * f * t) + 0.12 * exp(-12 * t) * sin(3.01 * f * t)
        return gain * edge(t, length, 0.013, 0.09) * exp(-2.9 * t) * texture

    return voice


def drone(note, length, gain):
    f = TAU * frequency(note)

    def voice(t):
        warmth = sin(f * t + 0.09 * sin(TAU * 0.7 * t)) + 0.23 * sin(2.005 * f * t)
        return gain * edge(t, length, 0.38, 0.62) * warmth

    return voice


def music(bpm, chords, melody, beat_pattern):
    beat = 60 / bpm
    samples = [0.0] * round(len(chords) * 4 * beat * RATE)
    for bar, chord in enumerate(chords):
        start = bar * 4 * beat
        for note in (chord[0] - 12, chord[1], chord[2]):
            length = 4.35 * beat
            pad = drone(note, length, 0.045)
            add(samples, start, length, pad, wrap=True)
        for place, degree in beat_pattern:
            length = 0.83
            add(samples, start + place * beat, length, pluck(chord[degree], length, 0.11), wrap=True)
        for place, note, beats in melody[bar]:
            length = beats * beat
            add(samples, start + place * beat, length, pluck(note, length, 0.18), wrap=True)
            add(samples, start + (place + 0.53) * beat, length, pluck(note, length, 0.034), wrap=True)
    return samples


def effect(length, seed, voice):
    random = Random(seed)
    samples = [0.0] * round(length * RATE)
    noise = 0.0
    for i in range(len(samples)):
        t = i / RATE
        impulse = random.uniform(-1, 1)
        noise += 0.22 * (impulse - noise)
        samples[i] = edge(t, length, 0.004, 0.027) * voice(t, noise, impulse - noise)
    return samples


def save(name, samples):
    # 统一保留峰值余量，使用所有目标平台都支持的单声道 PCM。
    pcm = array('h', (round(28500 * tanh(value)) for value in samples))
    if sys.byteorder != 'little':
        pcm.byteswap()
    with wave.open(str(OUTPUT / (name + '.wav')), 'wb') as file:
        file.setparams((1, 2, RATE, len(pcm), 'NONE', 'not compressed'))
        file.writeframes(pcm.tobytes())


def main():
    OUTPUT.mkdir(parents=True, exist_ok=True)
    save('dusk_gate', music(88,
        [(50, 57, 62), (46, 53, 60), (48, 55, 62), (45, 52, 57),
         (50, 57, 65), (46, 53, 62), (43, 50, 58), (45, 52, 60)],
        [[(0, 69, 1.2), (2, 65, 1), (3.25, 62, 0.7)], [(0.5, 65, 1.4), (2.5, 67, 1.2)],
         [(0, 69, 1.1), (1.5, 72, 0.9), (3, 67, 0.8)], [(1, 64, 1.5), (3, 60, 0.9)],
         [(0, 74, 1.4), (2, 69, 1.4)], [(0, 65, 0.9), (1.5, 67, 0.8), (3, 69, 0.9)],
         [(0.5, 70, 1.2), (2.5, 65, 1)], [(0, 64, 1), (2.5, 62, 2)]],
        [(0, 0), (1.5, 1), (2.5, 2), (3.5, 1)]))
    save('ember_path', music(104,
        [(45, 52, 60), (48, 55, 62), (43, 50, 59), (45, 52, 57),
         (41, 48, 57), (48, 55, 64), (43, 50, 62), (45, 52, 60)],
        [[(0.5, 64, 0.7), (1.5, 67, 0.6), (2.5, 69, 1.3)], [(0, 72, 1.2), (2, 67, 1.6)],
         [(0.5, 71, 0.7), (1.5, 67, 0.7), (3, 62, 0.9)], [(1, 64, 1.2), (3, 69, 1)],
         [(0, 72, 1.3), (2, 69, 0.8), (3, 65, 0.7)], [(0.5, 67, 0.7), (1.5, 72, 0.8), (3, 76, 1)],
         [(0, 74, 0.8), (1, 71, 0.8), (2.5, 67, 1.2)], [(0, 69, 1), (1.5, 64, 0.7), (3, 57, 1.45)]],
        [(0, 0), (0.75, 1), (1.5, 2), (2, 0), (2.75, 2), (3.5, 1)]))
    save('tap', effect(0.10, 13, lambda t, n, h: exp(-45 * t) * (0.32 * sin(TAU * (900 * t - 2100 * t * t)) + 0.11 * h)))
    save('swing', effect(0.19, 17, lambda t, n, h: sin(pi * t / 0.19) ** 1.3 * (0.48 * n + 0.15 * h + 0.10 * sin(TAU * (260 * t - 390 * t * t)))))
    save('spell', effect(0.32, 19, lambda t, n, h: exp(-5 * t) * (0.19 * sin(TAU * (410 * t + 820 * t * t)) + 0.11 * sin(TAU * (690 * t + 650 * t * t)) + 0.10 * n)))
    save('impact', effect(0.16, 23, lambda t, n, h: exp(-18 * t) * (0.42 * sin(TAU * (138 * t - 230 * t * t)) + 0.55 * n + 0.14 * h)))
    save('mend', effect(0.37, 29, lambda t, n, h: exp(-5.5 * t) * (0.22 * sin(TAU * 659.25 * t) + 0.14 * cos(TAU * 987.77 * t) + 0.09 * sin(TAU * 1318.5 * t))))


if __name__ == '__main__':
    main()
