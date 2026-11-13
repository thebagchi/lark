//go:build amd64 && !purego

#include "textflag.h"

// Hex encode and decode kernels. Plan 9 assembly, ABI0: the arguments are on
// the stack, and a kernel may clobber X15 and R14, which are scratch in ABI0.
//
// Every kernel does the same three things:
//   1. a loop over whole blocks;
//   2. the tail by one block that ends at the last byte and overlaps the block
//      before it. Hex is a pure function of the position, so the second write of
//      the same bytes is harmless, and no scalar loop or mask is needed. It does
//      not work in place, and the Go side never calls it so;
//   3. VZEROUPPER before RET, in a kernel that used YMM or ZMM registers.
// A kernel reads src[0:n] only and writes only the destination of that range.
// A decode kernel returns false when a digit is not hexadecimal. It does not
// find the digit: the Go side reports the error, which has no position.

// The digit table, and the constants that the kernels broadcast.
DATA hexlut<>+0(SB)/8, $"01234567"
DATA hexlut<>+8(SB)/8, $"89ABCDEF"
GLOBL hexlut<>(SB), RODATA|NOPTR, $16

DATA k0f<>(SB)/1, $0x0f
GLOBL k0f<>(SB), RODATA|NOPTR, $1
DATA k0f0f<>(SB)/2, $0x0f0f
GLOBL k0f0f<>(SB), RODATA|NOPTR, $2
DATA k30<>(SB)/1, $0x30
GLOBL k30<>(SB), RODATA|NOPTR, $1
DATA k09<>(SB)/1, $0x09
GLOBL k09<>(SB), RODATA|NOPTR, $1
DATA k20<>(SB)/1, $0x20
GLOBL k20<>(SB), RODATA|NOPTR, $1
DATA k61<>(SB)/1, $0x61
GLOBL k61<>(SB), RODATA|NOPTR, $1
DATA k05<>(SB)/1, $0x05
GLOBL k05<>(SB), RODATA|NOPTR, $1
DATA k0a<>(SB)/1, $0x0a
GLOBL k0a<>(SB), RODATA|NOPTR, $1
// Weights for the multiply-add of bytes: the even digit x16, the odd x1
// (little endian word 0x0110).
DATA kw<>(SB)/2, $0x0110
GLOBL kw<>(SB), RODATA|NOPTR, $2

// ---------------------------------------------------------------- SSSE3

// ENC16S turns the 16 bytes at S into 32 digits at D.
// X14 = digit table, X13 = 0x0f in every byte.
//   The high and low nibbles are split, interleaved (PUNPCK*BW: high nibble
//   first), and looked up with PSHUFB.
#define ENC16S(S, D) \
	MOVOU     (S), X0; \
	MOVO      X0, X1; \
	PSRLW     $4, X1; \
	PAND      X13, X1; \
	PAND      X13, X0; \
	MOVO      X1, X2; \
	PUNPCKLBW X0, X2; \
	PUNPCKHBW X0, X1; \
	MOVO      X14, X3; \
	PSHUFB    X2, X3; \
	MOVO      X14, X4; \
	PSHUFB    X1, X4; \
	MOVOU     X3, (D); \
	MOVOU     X4, 16(D)

// func _EncodeSSE(dst *byte, src *byte, n int)
// Requires: SSE2, SSSE3. n >= 16. dst holds 2*n bytes.
TEXT ·_EncodeSSE(SB), NOSPLIT, $0-24
	MOVQ  dst+0(FP), DI
	MOVQ  src+8(FP), SI
	MOVQ  n+16(FP), BX
	MOVOU hexlut<>(SB), X14
	MOVQ  $0x0f0f0f0f0f0f0f0f, AX
	MOVQ  AX, X13
	PUNPCKLQDQ X13, X13
	LEAQ  -16(SI)(BX*1), R9
	LEAQ  -16(BX), R10
	LEAQ  (DI)(R10*2), R11

sseloop:
	CMPQ SI, R9
	JAE  ssetail
	ENC16S(SI, DI)
	ADDQ $16, SI
	ADDQ $32, DI
	JMP  sseloop

ssetail:
	ENC16S(R9, R11)
	RET

// DEC16S reads 16 hex digits at S and leaves their values (garbage for a bad
// digit) in V, one per byte. X15 collects the valid mask (AND over all blocks).
// Constants: X8='0' X9=9 X10=0x20 X11='a' X12=5 X13=10.
//   digit:  d = c-'0'; valid when d <= 9 (min(d,9)==d).
//   letter: l = (c|0x20)-'a'; valid when l <= 5; value l+10.
#define DEC16S(OFF, S, V, T1, T2, T3) \
	MOVOU   OFF(S), V; \
	MOVO    V, T1; \
	PSUBB   X8, T1; \
	MOVO    T1, T2; \
	PMINUB  X9, T2; \
	PCMPEQB T1, T2; \
	POR     X10, V; \
	PSUBB   X11, V; \
	MOVO    V, T3; \
	PMINUB  X12, T3; \
	PCMPEQB V, T3; \
	PADDB   X13, V; \
	PAND    T3, V; \
	PAND    T2, T1; \
	POR     T1, V; \
	POR     T3, T2; \
	PAND    T2, X15

// SBYTES loads a byte constant into every byte of an XMM register.
#define SBYTES(IMM, X) \
	MOVQ       $IMM, AX; \
	MOVQ       AX, X; \
	PUNPCKLQDQ X, X

// func _DecodeSSE(dst *byte, src *byte, n int) bool
// Requires: SSE2, SSSE3. n >= 32 and even. dst holds n/2 bytes.
// Tail: the last 32 digits are decoded again at n-32.
TEXT ·_DecodeSSE(SB), NOSPLIT, $0-25
	MOVQ dst+0(FP), DI
	MOVQ src+8(FP), SI
	MOVQ n+16(FP), BX
	SBYTES(0x3030303030303030, X8)
	SBYTES(0x0909090909090909, X9)
	SBYTES(0x2020202020202020, X10)
	SBYTES(0x6161616161616161, X11)
	SBYTES(0x0505050505050505, X12)
	SBYTES(0x0a0a0a0a0a0a0a0a, X13)
	MOVQ       $0x0110011001100110, AX
	MOVQ       AX, X14
	PUNPCKLQDQ X14, X14
	PCMPEQB    X15, X15
	LEAQ -32(SI)(BX*1), R9
	LEAQ -32(BX), R10
	SHRQ $1, R10
	LEAQ (DI)(R10*1), R11

sdloop:
	CMPQ SI, R9
	JAE  sdtail
	DEC16S(0, SI, X0, X2, X3, X4)
	DEC16S(16, SI, X1, X5, X6, X7)
	PMADDUBSW X14, X0
	PMADDUBSW X14, X1
	PACKUSWB  X1, X0
	MOVOU     X0, (DI)
	ADDQ $32, SI
	ADDQ $16, DI
	JMP  sdloop

sdtail:
	DEC16S(0, R9, X0, X2, X3, X4)
	DEC16S(16, R9, X1, X5, X6, X7)
	PMADDUBSW X14, X0
	PMADDUBSW X14, X1
	PACKUSWB  X1, X0
	MOVOU     X0, (R11)
	PMOVMSKB  X15, AX
	CMPL      AX, $0xffff
	SETEQ     ret+24(FP)
	RET

// ---------------------------------------------------------------- AVX2
// The code of this section is the AVX2 kernel of poc/research/asm_kernels.

// ENC32 turns the 32 bytes at S into 64 hex digits at D.
// Y14 = digit table (both lanes), Y15 = 0x0f in every byte.
// T0..T3 are scratch.
//   hi nibbles and lo nibbles are split, interleaved per lane (VPUNPCK*BW),
//   looked up with VPSHUFB, and the two lanes are put back in order with
//   VPERM2I128.
#define ENC32(S, D, T0, T1, T2, T3) \
	VMOVDQU    (S), T0; \
	VPSRLW     $4, T0, T1; \
	VPAND      Y15, T1, T1; \
	VPAND      Y15, T0, T0; \
	VPUNPCKLBW T0, T1, T2; \
	VPUNPCKHBW T0, T1, T3; \
	VPSHUFB    T2, Y14, T2; \
	VPSHUFB    T3, Y14, T3; \
	VPERM2I128 $0x20, T3, T2, T0; \
	VPERM2I128 $0x31, T3, T2, T1; \
	VMOVDQU    T0, (D); \
	VMOVDQU    T1, 32(D)

// ENC16 is ENC32 on 16 bytes: 32 digits.
#define ENC16(S, D) \
	VMOVDQU    (S), X0; \
	VPSRLW     $4, X0, X1; \
	VPAND      X15, X1, X1; \
	VPAND      X15, X0, X0; \
	VPUNPCKLBW X0, X1, X2; \
	VPUNPCKHBW X0, X1, X3; \
	VPSHUFB    X2, X14, X2; \
	VPSHUFB    X3, X14, X3; \
	VMOVDQU    X2, (D); \
	VMOVDQU    X3, 16(D)

// func _EncodeAVX2(dst *byte, src *byte, n int)
// Requires: AVX, AVX2. n >= 16. dst holds 2*n bytes. Reads src[0:n] only and
// writes dst[0:2n] only. The tail is done by re-encoding the last full block
// (overlapping the one before): hex is a pure function of the position, so
// writing the same bytes twice is harmless and no scalar loop is needed.
TEXT ·_EncodeAVX2(SB), NOSPLIT, $0-24
	MOVQ         dst+0(FP), DI
	MOVQ         src+8(FP), SI
	MOVQ         n+16(FP), BX
	VBROADCASTI128 hexlut<>(SB), Y14
	VPBROADCASTB k0f<>(SB), Y15
	CMPQ         BX, $32
	JB           lt32
	LEAQ         -32(SI)(BX*1), R9
	LEAQ         -32(BX), R10
	LEAQ         (DI)(R10*2), R11

loop:
	CMPQ SI, R9
	JAE  last
	ENC32(SI, DI, Y0, Y1, Y2, Y3)
	ADDQ $32, SI
	ADDQ $64, DI
	JMP  loop

last:
	ENC32(R9, R11, Y0, Y1, Y2, Y3)
	VZEROUPPER
	RET

lt32:
	LEAQ -16(SI)(BX*1), R9
	LEAQ -16(BX), R10
	LEAQ (DI)(R10*2), R11
	ENC16(SI, DI)
	ENC16(R9, R11)
	VZEROUPPER
	RET

// DEC32 reads 32 hex digits at S and leaves their values (0 for a bad digit)
// in V, one per byte. Y15 collects the valid mask (AND over all blocks).
// Constants: Y8='0' Y9=9 Y10=0x20 Y11='a' Y12=5 Y13=10.
//   digit: d = c-'0'; valid when d <= 9 (min(d,9)==d).
//   letter: l = (c|0x20)-'a'; valid when l <= 5; value l+10.
#define DEC32(OFF, S, V, T1, T2, T3) \
	VMOVDQU  OFF(S), V; \
	VPSUBB   Y8, V, T1; \
	VPMINUB  Y9, T1, T2; \
	VPCMPEQB T2, T1, T2; \
	VPOR     Y10, V, V; \
	VPSUBB   Y11, V, V; \
	VPMINUB  Y12, V, T3; \
	VPCMPEQB T3, V, T3; \
	VPADDB   Y13, V, V; \
	VPAND    T3, V, V; \
	VPAND    T2, T1, T1; \
	VPOR     T1, V, V; \
	VPOR     T3, T2, T2; \
	VPAND    T2, Y15, Y15

// PACK2 packs the values of two blocks (A then B) into 32 bytes at D.
// Y14 = weights (16, 1).
#define PACK2(A, B, D) \
	VPMADDUBSW Y14, A, A; \
	VPMADDUBSW Y14, B, B; \
	VPACKUSWB  B, A, A; \
	VPERMQ     $0xD8, A, A; \
	VMOVDQU    A, (D)

// func _DecodeAVX2(dst *byte, src *byte, n int) bool
// Requires: AVX, AVX2. n >= 32 and even. dst holds n/2 bytes. Returns false
// when any digit is not hex (the caller then decodes again with the checked
// path to get the exact error). Reads src[0:n] only; writes dst[0:n/2] only.
// Tail: the last 32 digits are decoded again at n-32 (same bytes written).
TEXT ·_DecodeAVX2(SB), NOSPLIT, $0-25
	MOVQ         dst+0(FP), DI
	MOVQ         src+8(FP), SI
	MOVQ         n+16(FP), BX
	VPBROADCASTB k30<>(SB), Y8
	VPBROADCASTB k09<>(SB), Y9
	VPBROADCASTB k20<>(SB), Y10
	VPBROADCASTB k61<>(SB), Y11
	VPBROADCASTB k05<>(SB), Y12
	VPBROADCASTB k0a<>(SB), Y13
	VPBROADCASTW kw<>(SB), Y14
	VPCMPEQB     Y15, Y15, Y15
	LEAQ         -32(SI)(BX*1), R9
	LEAQ         -32(BX), R10
	SHRQ         $1, R10
	LEAQ         (DI)(R10*1), R11

loop64:
	CMPQ BX, $64
	JB   rest
	DEC32(0, SI, Y0, Y2, Y3, Y4)
	DEC32(32, SI, Y1, Y5, Y6, Y7)
	PACK2(Y0, Y1, DI)
	ADDQ $64, SI
	ADDQ $32, DI
	SUBQ $64, BX
	JMP  loop64

rest:
	TESTQ BX, BX
	JZ    done
	CMPQ  BX, $32
	JBE   final
	DEC32(0, SI, Y0, Y2, Y3, Y4)
	VPMADDUBSW Y14, Y0, Y0
	VPACKUSWB  Y0, Y0, Y0
	VPERMQ     $0x08, Y0, Y0
	VMOVDQU    X0, (DI)

final:
	DEC32(0, R9, Y0, Y2, Y3, Y4)
	VPMADDUBSW Y14, Y0, Y0
	VPACKUSWB  Y0, Y0, Y0
	VPERMQ     $0x08, Y0, Y0
	VMOVDQU    X0, (R11)

done:
	VPMOVMSKB Y15, AX
	NOTL      AX
	TESTL     AX, AX
	SETEQ     ret+24(FP)
	VZEROUPPER
	RET

// ---------------------------------------------------------------- AVX-512 F, BW
// A kernel here uses the 64-byte registers, and the mask registers for the digit
// checks. Z13 and Z14 hold constants. VPMOVZXBW and VPMOVWB, both BW, make
// the lane fix-up of the AVX2 kernels unnecessary: the widen and the narrow are
// whole-register, in order.

// ENC512 turns the 32 bytes at S into 64 digits at D.
// Z14 = digit table (in every 128-bit lane), Z13 = 0x0f0f in every word.
//   Each byte b is widened to a word. The word holds b>>4 in its low byte and b
//   in its high byte; the mask leaves the two nibbles; PSHUFB looks them up
//   inside each 128-bit lane, and the table is the same in every lane.
#define ENC512(S, D) \
	VPMOVZXBW (S), Z0; \
	VPSRLW    $4, Z0, Z1; \
	VPSLLW    $8, Z0, Z2; \
	VPORQ     Z1, Z2, Z1; \
	VPANDQ    Z13, Z1, Z1; \
	VPSHUFB   Z1, Z14, Z1; \
	VMOVDQU64 Z1, (D)

// func _EncodeAVX512(dst *byte, src *byte, n int)
// Requires: AVX512F, AVX512BW. n >= 32. dst holds 2*n bytes.
TEXT ·_EncodeAVX512(SB), NOSPLIT, $0-24
	MOVQ            dst+0(FP), DI
	MOVQ            src+8(FP), SI
	MOVQ            n+16(FP), BX
	VBROADCASTI32X4 hexlut<>(SB), Z14
	VPBROADCASTW    k0f0f<>(SB), Z13
	LEAQ            -32(SI)(BX*1), R9
	LEAQ            -32(BX), R10
	LEAQ            (DI)(R10*2), R11

zeloop:
	CMPQ SI, R9
	JAE  zetail
	ENC512(SI, DI)
	ADDQ $32, SI
	ADDQ $64, DI
	JMP  zeloop

zetail:
	ENC512(R9, R11)
	VZEROUPPER
	RET

// DEC512 reads 64 hex digits at S, and writes their 32 bytes to D. K7 collects
// the valid mask (AND over all blocks). Constants: Z8='0' Z9=9 Z10=0x20 Z11='a'
// Z12=5 Z13=10 Z14=weights (16, 1).
//   d = c-'0' is the value of a digit. l = (c|0x20)-'a' is a letter when
//   l <= 5, and l+10 is its value: that add is merged into d under the letter
//   mask, so d holds the value for a digit or a letter. A byte that is neither
//   has a garbage value, and the valid mask refuses the whole input.
#define DEC512(S, D) \
	VMOVDQU64   (S), Z0; \
	VPSUBB      Z8, Z0, Z1; \
	VPMINUB     Z9, Z1, Z2; \
	VPCMPEQB    Z2, Z1, K1; \
	VPORQ       Z10, Z0, Z0; \
	VPSUBB      Z11, Z0, Z0; \
	VPMINUB     Z12, Z0, Z3; \
	VPCMPEQB    Z3, Z0, K2; \
	VPADDB      Z13, Z0, K2, Z1; \
	KORQ        K1, K2, K1; \
	KANDQ       K1, K7, K7; \
	VPMADDUBSW  Z14, Z1, Z1; \
	VPMOVWB     Z1, Y2; \
	VMOVDQU     Y2, (D)

// func _DecodeAVX512(dst *byte, src *byte, n int) bool
// Requires: AVX512F, AVX512BW. n >= 64 and even. dst holds n/2 bytes.
// Tail: the last 64 digits are decoded again at n-64.
TEXT ·_DecodeAVX512(SB), NOSPLIT, $0-25
	MOVQ         dst+0(FP), DI
	MOVQ         src+8(FP), SI
	MOVQ         n+16(FP), BX
	VPBROADCASTB k30<>(SB), Z8
	VPBROADCASTB k09<>(SB), Z9
	VPBROADCASTB k20<>(SB), Z10
	VPBROADCASTB k61<>(SB), Z11
	VPBROADCASTB k05<>(SB), Z12
	VPBROADCASTB k0a<>(SB), Z13
	VPBROADCASTW kw<>(SB), Z14
	KXNORQ       K7, K7, K7
	LEAQ         -64(SI)(BX*1), R9
	LEAQ         -64(BX), R10
	SHRQ         $1, R10
	LEAQ         (DI)(R10*1), R11

zdloop:
	CMPQ SI, R9
	JAE  zdtail
	DEC512(SI, DI)
	ADDQ $64, SI
	ADDQ $32, DI
	JMP  zdloop

zdtail:
	DEC512(R9, R11)
	KMOVQ  K7, AX
	NOTQ   AX
	TESTQ  AX, AX
	SETEQ  ret+24(FP)
	VZEROUPPER
	RET
