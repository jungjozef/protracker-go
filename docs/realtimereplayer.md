How digital audio works

Sound = pressure waves

Speaker cone moves back/forth → pushes air → ear drum vibrates → you hear. Faster vibration = higher pitch. Bigger movement = louder.

Sampling: continuous → numbers

Microphone measures air pressure thousands of times per second. Each measurement = one sample (a number).

pressure                                                                                                                                                                                                                
│  ╭─╮     ╭─╮
│ ╭╯ ╰╮   ╭╯ ╰╮                                                                                                                                                                                                     
──┼─╯    ╰───╯    ╰──  time                                                                                                                                                                                             
│                                                                                                                                                                                                                     
↑ ↑ ↑ ↑ ↑ ↑ ↑ ↑   ← sample points

Sample rate = how many samples per second. 44100 Hz = 44100 snapshots/sec. More = higher fidelity ceiling.

Nyquist theorem

To represent frequency F, need sample rate ≥ 2×F. Human hearing tops ~20kHz. So 44100 Hz covers full range (44100/2 = 22050 Hz). That's why 44100 is the standard.

Bit depth = precision per sample

Each sample = integer. More bits = finer steps = less quantization noise.

- 8-bit: 256 levels. Gritty, noisy. ← Amiga/ProTracker uses this
- 16-bit: 65536 levels. CD quality. Clean.
- 32-bit float: ~16 million effective levels. Used internally in DSP.

ProTracker samples are signed 8-bit (int8, -128..127). Your code divides by 128.0 to get -1.0..1.0 float range. That's the conversion.

Stereo = two channels

Two streams: Left + Right, interleaved in memory:

[L0, R0, L1, R1, L2, R2, ...]

Your current code does exactly this — out[i*2] = left, out[i*2+1] = right.

The audio signal chain

Your code            OS / Driver            Hardware                                                                                                                                                                  
──────────           ──────────             ────────                                                                                                                                                                    
[]float64  →  DAC buffer  →  D/A converter  →  speaker
(numbers)     (ring buffer)   (digital→analog)  (sound)

DAC = Digital-to-Analog Converter. Turns your numbers into voltage. Voltage drives speaker coil.

What the OS audio system does

OS has audio hardware on a real-time clock. Every N samples it needs fresh data. It either:

1. Calls your callback (pull model) — "give me 512 samples NOW"
2. Reads from a buffer you fill (push model) — you write to a pipe/writer, OS drains it

Both models exist. Libraries like oto expose push (io.Writer). portaudio exposes callback.

Buffer size tradeoff

Small buffer (e.g. 256 samples @ 44100 = 5.8ms)                                                                                                                                                                       
→ Low latency (good for interactive)                                                                                                                                                                                  
→ Less margin → any CPU hiccup = dropout

Large buffer (e.g. 8192 samples @ 44100 = 185ms)                                                                                                                                                                        
→ High latency (bad for interactive)                                                                                                                                                                                  
→ Big margin → GC pauses, scheduling jitter absorbed

For a mod player (no interactive input latency needed): use big buffer, no dropouts.

Mixing = addition

Multiple voices playing simultaneously = add their samples together:

output = voice0 + voice1 + voice2 + voice3

If all 4 voices full volume at same time, sum can exceed -1..1 range → clipping (distortion). Solution: scale down (* 0.25) or soft-clip. Your current code doesn't clip — works because ProTracker songs rarely have   
all 4 voices maxed simultaneously.

Volume = multiply

sample * volume   // volume 0.0..1.0

That's it. Halve amplitude = 6dB quieter perceptually.

Pitch = playback speed

ProTracker sample data is recorded at some base rate. To play it at different pitch, change how fast you advance through the sample data. Your delta = paulaClk / (period * outputRate). If delta > 1.0, you skip       
samples (higher pitch). If delta < 1.0, you repeat samples (lower pitch).

delta = 0.5 → read every other output sample from same input position → half speed → octave lower
delta = 2.0 → advance 2 input samples per output sample → double speed → octave higher
                                                                                                                                                                                                                          
---                                                                                                                                                                                                                     
Chain summary:

sample data (int8[])                                                                                                                                                                                                  
→ advance phase by delta each output sample                                                                                                                                                                           
→ multiply by volume                                                                                                                                                                                                  
→ add 4 voices together                                                                                                                                                                                               
→ write L/R float pairs into buffer                                                                                                                                                                                   
→ OS sends buffer to DAC                                                                                                                                                                                              
→ DAC → speaker → air → ear

That's the whole thing. Your renderTick() already does all of it correctly.

❯ I know the physics of audio. I need to learn specifically how the audio replayer should work on a computer.

⏺ Audio on computer: software side

OS audio stack (macOS = Core Audio)

Your program                                                                                                                                                                                                            
↓                                                                                                                                                                                                                   
Audio library (oto / portaudio)                                                                                                                                                                                       
↓                                                                                                                                                                                                                   
Core Audio (macOS) / ALSA+PulseAudio (Linux) / WASAPI (Windows)
↓                                                                                                                                                                                                                   
Kernel audio driver
↓                                                                                                                                                                                                                   
DAC hardware

You never talk to hardware directly. OS owns it.

What OS does

OS runs audio hardware on hardware interrupt — fixed clock, not your scheduler. Every N samples, hardware fires interrupt → kernel wakes audio thread → needs data now. Miss the deadline = xrun = audible              
glitch/silence.

This thread is real-time priority. GC, scheduler, mutexes = dangerous here.

Two models exposed to you

1. Callback (pull)

OS calls your function on audio thread:                                                                                                                                                                                 
func callback(out []float32) {
// fill out with N samples                                                                                                                                                                                          
// MUST return before next interrupt
// NO: allocations, locks, syscalls, println                                                                                                                                                                        
}                                               
PortAudio works this way.

2. Writer (push)

You write to an io.Writer. Library has internal ring buffer. Audio thread drains it independently:                                                                                                                      
player.Write(samples)  // blocks when buffer full
oto works this way. Simpler. GC-safe on your side.

Ring buffer: the core primitive

[  consumed  |  ready to play  |  empty  ]                                                                                                                                                                              
↑                 ↑                                                                                                                                                                                        
read ptr          write ptr

Your goroutine writes →→→→→→                                                                                                                                                                                            
Audio thread reads →→→→→→

- Write ptr chases read ptr
- If write ptr catches read ptr = full → your goroutine blocks/waits
- If read ptr catches write ptr = underrun → output silence, log xrun

What your replayer goroutine does

for !r.done {                                                                                                                                                                                                         
samples := renderTick(r)        // produce chunk (variable size!)                                                                                                                                                 
player.Write(samplesToBytes(samples))  // blocks when buffer full                                                                                                                                                   
}

renderTick already returns variable-length slices (CIA accumulator). That's fine — Write handles arbitrary sizes.

oto specifically (simplest Go option)

ctx, _ := oto.NewContext(44100, 2, 2)  // sampleRate, channels, bytesPerSample                                                                                                                                        
player := ctx.NewPlayer(reader)         // reader = your io.Reader                                                                                                                                                      
player.Play()

Or use oto.NewPlayer with an io.Reader you implement:                                                                                                                                                                   
func (r *Replayer) Read(buf []byte) (int, error) {                                                                                                                                                                    
// oto calls this when it needs more data                                                                                                                                                                           
// render enough ticks to fill buf                                                                                                                                                                                  
// convert float64 → int16 → bytes                                                                                                                                                                                
}

oto calls Read on its own goroutine. You render inside Read. Simple, no explicit ring buffer needed — oto provides it.

Sample format conversion

Your code uses float64. Hardware wants int16 (CD quality) or float32. Convert at the boundary:

// float64 [-1,1] → int16                                                                                                                                                                                               
func f64ToI16(s float64) int16 {
s = max(-1.0, min(1.0, s))  // clamp                                                                                                                                                                                
return int16(s * 32767)                                                                                                                                                                                             
}

// then pack into []byte (little-endian)                                                                                                                                                                                
binary.LittleEndian.PutUint16(buf[i:], uint16(sample))

Latency

latency = bufferSize / sampleRate

Buffer = 4096 samples → 4096/44100 ≈ 93ms. You hear note 93ms after replayer triggers it. Fine for mod player. Bad for instruments.

Thread model for mod player

main goroutine:   UI, input, load module                                                                                                                                                                              
replayer goroutine: renderTick loop, writes to oto player                                                                                                                                                               
oto internal goroutine: calls Read(), sends to Core Audio                                                                                                                                                               
Core Audio thread: sends to DAC (you never touch this)

No shared state between replayer and oto internals except the io.Reader interface. Safe.

Practical starting point

import "github.com/ebitengine/oto/v3"

type ModReader struct { r *replayerState; buf []float64 }

func (m *ModReader) Read(p []byte) (int, error) {                                                                                                                                                                       
need := len(p) / 4  // 2 channels × 2 bytes (int16)                                                                                                                                                               
for len(m.buf) < need*2 {                                                                                                                                                                                           
m.buf = append(m.buf, renderTick(m.r)...)
}                                                                                                                                                                                                                   
for i := 0; i < need; i++ {
l := f64ToI16(m.buf[i*2])                                                                                                                                                                                       
r := f64ToI16(m.buf[i*2+1])
binary.LittleEndian.PutUint16(p[i*4:],   uint16(l))                                                                                                                                                             
binary.LittleEndian.PutUint16(p[i*4+2:], uint16(r))
}                                                                                                                                                                                                                   
m.buf = m.buf[need*2:]
return need * 4, nil                                                                                                                                                                                                
}

Oto calls Read → you call renderTick → samples flow out. Done.                                                                                                                                                          
           