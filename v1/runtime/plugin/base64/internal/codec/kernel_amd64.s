//go:build amd64 && !purego

#include "textflag.h"

// Base64 encode and decode kernels. Plan 9 assembly, ABI0: the arguments are on
// the stack, and a kernel may clobber X15 and R14, which are scratch in ABI0.
//
// The method is the one of Muła and Lemire, "Faster Base64 Encoding and
// Decoding Using AVX2 Instructions" (arXiv 1704.00605), taken to 128 and 512
// bits. Encode: a byte shuffle puts the three bytes of a group in the four
// positions that each sextet is cut from; two multiplies cut the four sextets;
// a subtract, a compare and one shuffle turn each sextet into its letter.
// Decode: the high and the low nibble of a character each look up a bit set,
// and the AND of the two is zero for exactly the characters of the alphabet; a
// third lookup gives the offset that turns a character into its sextet; two
// multiply-adds join four sextets in 24 bits; a shuffle drops the pad byte.
//
// Every kernel loops over whole blocks and returns how much it handled. It does
// not do a tail: the Go side hands what is left to encoding/base64, which also
// does the padding and the strict check of the last group. A kernel reads and
// writes only what its loop test allows, so it is safe at the last byte of a
// slice:
//   - an encode block reads 4 bytes more than it uses (the load of the second
//     half starts 12 bytes in and is 16 bytes long) and the loop test counts
//     that read; it writes exactly the characters of the bytes it used;
//   - a decode block reads exactly its characters and writes 4 bytes more than
//     it decodes (a 16 byte store of 12 bytes), and the loop test counts the
//     room that the caller has for that store.
// A decode kernel returns -1 when a character is not in the alphabet. It does
// not find the character: the error text names the whole text.
//
// A kernel takes a _Run (kernel.go): Size is n, the bytes or characters to
// read; Room is the bytes of dst that a decode kernel may write; Alphabet is the
// index of a record of b64tab. 0 is the standard alphabet of RFC 4648 section 4,
// 1 is the URL and filename safe one of section 5. A record has 80 bytes, at
// these offsets:
//   0  encode: the offset that is added to a sextet, by class of the sextet
//   16 decode: the bit set of the low nibble
//   32 decode: the bit set of the high nibble
//   48 decode: the offset that turns a character into its sextet, by class
//   64 the one character whose class is moved, and the value that moves it
// The record is made from the alphabets by a script that is not shipped, and a
// test checks all 256 byte values against encoding/base64 for each alphabet.

DATA b64tab<>+0(SB)/8, $0xfcfcfcfcfcfcfc47
DATA b64tab<>+8(SB)/8, $0x000041f0edfcfcfc
DATA b64tab<>+16(SB)/8, $0x1111111111111135
DATA b64tab<>+24(SB)/8, $0x5a5b5b5b5a131111
DATA b64tab<>+32(SB)/8, $0x4020080402011010
DATA b64tab<>+40(SB)/8, $0x1010101010101010
DATA b64tab<>+48(SB)/8, $0xb9b9bfbf04131000
DATA b64tab<>+56(SB)/8, $0x0000000000000000
DATA b64tab<>+64(SB)/8, $0x000000000000ff2f
DATA b64tab<>+72(SB)/8, $0x0000000000000000
DATA b64tab<>+80(SB)/8, $0xfcfcfcfcfcfcfc47
DATA b64tab<>+88(SB)/8, $0x00004120effcfcfc
DATA b64tab<>+96(SB)/8, $0x1111111111111135
DATA b64tab<>+104(SB)/8, $0x535b5a5b5b131111
DATA b64tab<>+112(SB)/8, $0x4020080402011010
DATA b64tab<>+120(SB)/8, $0x1010101010101010
DATA b64tab<>+128(SB)/8, $0xb9b9bfbf04110000
DATA b64tab<>+136(SB)/8, $0x0000e00000000000
DATA b64tab<>+144(SB)/8, $0x000000000000085f
DATA b64tab<>+152(SB)/8, $0x0000000000000000
GLOBL b64tab<>(SB), RODATA|NOPTR, $160

// The shuffle that puts the bytes of a group of three, (a b c), in the order
// (b a c b) that the multiplies cut the sextets from.
DATA b64shuf<>+0(SB)/8, $0x0405030401020001
DATA b64shuf<>+8(SB)/8, $0x0a0b090a07080607
GLOBL b64shuf<>(SB), RODATA|NOPTR, $16

// The shuffle that takes 3 bytes of each dword (high byte first) to 12 bytes,
// and zeroes the last 4 (bit 7 set).
DATA b64pack<>+0(SB)/8, $0x090a040506000102
DATA b64pack<>+8(SB)/8, $0x808080800c0d0e08
GLOBL b64pack<>(SB), RODATA|NOPTR, $16

// Q64 puts a 64 bit constant in the low half of an XMM register.
#define Q64(IMM, X) \
	MOVQ $IMM, AX; \
	MOVQ AX, X

// VQ64 is Q64 in the VEX encoding, for the kernels that use YMM or ZMM: a
// legacy SSE instruction next to dirty upper halves costs a merge.
#define VQ64(IMM, X) \
	MOVQ  $IMM, AX; \
	VMOVQ AX, X

// TAB sets R8 to the record of the alphabet in R8.
#define TAB \
	LEAQ   b64tab<>(SB), R9; \
	IMUL3Q $80, R8, R8; \
	ADDQ   R9, R8

// ---------------------------------------------------------------- SSSE3

// func _EncodeSSE(dst *byte, src *byte, run *_Run) int
// Requires: SSE2, SSSE3. Handles 12 bytes of src and 16 of dst a step, while 16
// bytes remain. Returns how many bytes of src it handled.
TEXT ·_EncodeSSE(SB), NOSPLIT, $0-32
	MOVQ  dst+0(FP), DI
	MOVQ  src+8(FP), SI
	MOVQ  run+16(FP), R11
	MOVQ  (R11), BX
	MOVQ  16(R11), R8
	MOVQ  BX, R10
	TAB
	MOVOU (R8), X15
	MOVOU b64shuf<>(SB), X8
	Q64(0x0fc0fc000fc0fc00, X9)
	PUNPCKLQDQ X9, X9
	Q64(0x0400004004000040, X10)
	PUNPCKLQDQ X10, X10
	Q64(0x003f03f0003f03f0, X11)
	PUNPCKLQDQ X11, X11
	Q64(0x0100001001000010, X12)
	PUNPCKLQDQ X12, X12
	Q64(0x3333333333333333, X13)
	PUNPCKLQDQ X13, X13
	Q64(0x1a1a1a1a1a1a1a1a, X14)
	PUNPCKLQDQ X14, X14
	Q64(0x0d0d0d0d0d0d0d0d, X7)
	PUNPCKLQDQ X7, X7

esloop:
	CMPQ    BX, $16
	JB      esdone
	MOVOU   (SI), X0
	PSHUFB  X8, X0
	MOVO    X0, X1
	PAND    X9, X1
	PMULHUW X10, X1
	PAND    X11, X0
	PMULLW  X12, X0
	POR     X1, X0
	MOVO    X0, X1
	PSUBUSB X13, X1
	MOVO    X14, X2
	PCMPGTB X0, X2
	PAND    X7, X2
	POR     X2, X1
	MOVO    X15, X3
	PSHUFB  X1, X3
	PADDB   X0, X3
	MOVOU   X3, (DI)
	ADDQ    $12, SI
	ADDQ    $16, DI
	SUBQ    $12, BX
	JMP     esloop

esdone:
	SUBQ BX, R10
	MOVQ R10, ret+24(FP)
	RET

// func _DecodeSSE(dst *byte, src *byte, run *_Run) int
// Requires: SSE2, SSSE3. Handles 16 characters of src and 12 bytes of dst a
// step, while 16 characters remain and the room is 16 bytes (the store is 16
// bytes). Returns how many characters it handled, or -1 for one that is not in
// the alphabet.
TEXT ·_DecodeSSE(SB), NOSPLIT, $0-32
	MOVQ    dst+0(FP), DI
	MOVQ    src+8(FP), SI
	MOVQ    run+16(FP), R11
	MOVQ    (R11), BX
	MOVQ    8(R11), CX
	MOVQ    16(R11), R8
	MOVQ    BX, R10
	TAB
	MOVOU   16(R8), X8
	MOVOU   32(R8), X9
	MOVOU   48(R8), X10
	PXOR    X1, X1
	MOVBLZX 64(R8), AX
	MOVQ    AX, X11
	PSHUFB  X1, X11
	MOVBLZX 65(R8), AX
	MOVQ    AX, X12
	PSHUFB  X1, X12
	Q64(0x0f0f0f0f0f0f0f0f, X13)
	PUNPCKLQDQ X13, X13
	Q64(0x0140014001400140, X14)
	PUNPCKLQDQ X14, X14
	Q64(0x0001100000011000, X15)
	PUNPCKLQDQ X15, X15
	MOVOU   b64pack<>(SB), X7
	PXOR    X6, X6

dsloop:
	CMPQ      BX, $16
	JB        dsdone
	CMPQ      CX, $16
	JB        dsdone
	MOVOU     (SI), X0
	MOVO      X0, X1
	PSRLW     $4, X1
	PAND      X13, X1
	MOVO      X0, X2
	PAND      X13, X2
	MOVO      X8, X3
	PSHUFB    X2, X3
	MOVO      X9, X4
	PSHUFB    X1, X4
	PAND      X3, X4
	POR       X4, X6
	MOVO      X0, X2
	PCMPEQB   X11, X2
	PAND      X12, X2
	PADDB     X1, X2
	MOVO      X10, X3
	PSHUFB    X2, X3
	PADDB     X3, X0
	PMADDUBSW X14, X0
	PMADDWL   X15, X0
	PSHUFB    X7, X0
	MOVOU     X0, (DI)
	ADDQ      $16, SI
	ADDQ      $12, DI
	SUBQ      $16, BX
	SUBQ      $12, CX
	JMP       dsloop

dsdone:
	PXOR     X5, X5
	PCMPEQB  X6, X5
	PMOVMSKB X5, AX
	CMPL     AX, $0xffff
	JNE      dsbad
	SUBQ     BX, R10
	MOVQ     R10, ret+24(FP)
	RET

dsbad:
	MOVQ $-1, ret+24(FP)
	RET

// ---------------------------------------------------------------- AVX2

// func _EncodeAVX2(dst *byte, src *byte, run *_Run) int
// Requires: AVX, AVX2. Handles 24 bytes of src and 32 of dst a step, while 28
// bytes remain (the second load starts 12 in and is 16 long). Returns how many
// bytes of src it handled.
TEXT ·_EncodeAVX2(SB), NOSPLIT, $0-32
	MOVQ dst+0(FP), DI
	MOVQ src+8(FP), SI
	MOVQ run+16(FP), R11
	MOVQ (R11), BX
	MOVQ 16(R11), R8
	MOVQ BX, R10
	TAB
	VBROADCASTI128 (R8), Y15
	VBROADCASTI128 b64shuf<>(SB), Y8
	VQ64(0x0fc0fc000fc0fc00, X9)
	VPBROADCASTQ X9, Y9
	VQ64(0x0400004004000040, X10)
	VPBROADCASTQ X10, Y10
	VQ64(0x003f03f0003f03f0, X11)
	VPBROADCASTQ X11, Y11
	VQ64(0x0100001001000010, X12)
	VPBROADCASTQ X12, Y12
	VQ64(0x3333333333333333, X13)
	VPBROADCASTQ X13, Y13
	VQ64(0x1a1a1a1a1a1a1a1a, X14)
	VPBROADCASTQ X14, Y14
	VQ64(0x0d0d0d0d0d0d0d0d, X7)
	VPBROADCASTQ X7, Y7

e2loop:
	CMPQ        BX, $28
	JB          e2done
	VMOVDQU     (SI), X0
	VINSERTI128 $1, 12(SI), Y0, Y0
	VPSHUFB     Y8, Y0, Y0
	VPAND       Y9, Y0, Y1
	VPMULHUW    Y10, Y1, Y1
	VPAND       Y11, Y0, Y2
	VPMULLW     Y12, Y2, Y2
	VPOR        Y1, Y2, Y0
	VPSUBUSB    Y13, Y0, Y1
	VPCMPGTB    Y0, Y14, Y2
	VPAND       Y7, Y2, Y2
	VPOR        Y2, Y1, Y1
	VPSHUFB     Y1, Y15, Y1
	VPADDB      Y0, Y1, Y1
	VMOVDQU     Y1, (DI)
	ADDQ        $24, SI
	ADDQ        $32, DI
	SUBQ        $24, BX
	JMP         e2loop

e2done:
	VZEROUPPER
	SUBQ BX, R10
	MOVQ R10, ret+24(FP)
	RET

// func _DecodeAVX2(dst *byte, src *byte, run *_Run) int
// Requires: AVX, AVX2. Handles 32 characters of src and 24 bytes of dst a
// step, while 32 characters remain and the room is 28 bytes (two stores of 16
// bytes, 12 bytes apart). Returns how many characters it handled, or -1.
TEXT ·_DecodeAVX2(SB), NOSPLIT, $0-32
	MOVQ dst+0(FP), DI
	MOVQ src+8(FP), SI
	MOVQ run+16(FP), R11
	MOVQ (R11), BX
	MOVQ 8(R11), CX
	MOVQ 16(R11), R8
	MOVQ BX, R10
	TAB
	VBROADCASTI128 16(R8), Y8
	VBROADCASTI128 32(R8), Y9
	VBROADCASTI128 48(R8), Y10
	VPBROADCASTB 64(R8), Y11
	VPBROADCASTB 65(R8), Y12
	VQ64(0x0f0f0f0f0f0f0f0f, X13)
	VPBROADCASTQ X13, Y13
	VQ64(0x0140014001400140, X14)
	VPBROADCASTQ X14, Y14
	VQ64(0x0001100000011000, X15)
	VPBROADCASTQ X15, Y15
	VBROADCASTI128 b64pack<>(SB), Y7
	VPXOR Y6, Y6, Y6

d2loop:
	CMPQ         BX, $32
	JB           d2done
	CMPQ         CX, $28
	JB           d2done
	VMOVDQU      (SI), Y0
	VPSRLW       $4, Y0, Y1
	VPAND        Y13, Y1, Y1
	VPAND        Y13, Y0, Y2
	VPSHUFB      Y2, Y8, Y2
	VPSHUFB      Y1, Y9, Y3
	VPAND        Y2, Y3, Y3
	VPOR         Y3, Y6, Y6
	VPCMPEQB     Y11, Y0, Y2
	VPAND        Y12, Y2, Y2
	VPADDB       Y1, Y2, Y2
	VPSHUFB      Y2, Y10, Y2
	VPADDB       Y2, Y0, Y0
	VPMADDUBSW   Y14, Y0, Y0
	VPMADDWD     Y15, Y0, Y0
	VPSHUFB      Y7, Y0, Y0
	VMOVDQU      X0, (DI)
	VEXTRACTI128 $1, Y0, X1
	VMOVDQU      X1, 12(DI)
	ADDQ         $32, SI
	ADDQ         $24, DI
	SUBQ         $32, BX
	SUBQ         $24, CX
	JMP          d2loop

d2done:
	VPTEST Y6, Y6
	JNZ    d2bad
	VZEROUPPER
	SUBQ   BX, R10
	MOVQ   R10, ret+24(FP)
	RET

d2bad:
	VZEROUPPER
	MOVQ $-1, ret+24(FP)
	RET

// ---------------------------------------------------------------- AVX-512

// func _EncodeAVX512(dst *byte, src *byte, run *_Run) int
// Requires: AVX512F, AVX512BW. Handles 48 bytes of src and 64 of dst a step,
// while 52 bytes remain (the last load starts 36 in and is 16 long). Returns
// how many bytes of src it handled.
TEXT ·_EncodeAVX512(SB), NOSPLIT, $0-32
	MOVQ dst+0(FP), DI
	MOVQ src+8(FP), SI
	MOVQ run+16(FP), R11
	MOVQ (R11), BX
	MOVQ 16(R11), R8
	MOVQ BX, R10
	TAB
	VBROADCASTI32X4 (R8), Z15
	VBROADCASTI32X4 b64shuf<>(SB), Z8
	VQ64(0x0fc0fc000fc0fc00, X9)
	VPBROADCASTQ X9, Z9
	VQ64(0x0400004004000040, X10)
	VPBROADCASTQ X10, Z10
	VQ64(0x003f03f0003f03f0, X11)
	VPBROADCASTQ X11, Z11
	VQ64(0x0100001001000010, X12)
	VPBROADCASTQ X12, Z12
	VQ64(0x3333333333333333, X13)
	VPBROADCASTQ X13, Z13
	VQ64(0x1a1a1a1a1a1a1a1a, X14)
	VPBROADCASTQ X14, Z14
	VQ64(0x0d0d0d0d0d0d0d0d, X7)
	VPBROADCASTQ X7, Z7

e5loop:
	CMPQ         BX, $52
	JB           e5done
	VMOVDQU      (SI), X0
	VINSERTI128  $1, 12(SI), Y0, Y0
	VMOVDQU      24(SI), X1
	VINSERTI128  $1, 36(SI), Y1, Y1
	VINSERTI64X4 $1, Y1, Z0, Z0
	VPSHUFB      Z8, Z0, Z0
	VPANDD       Z9, Z0, Z1
	VPMULHUW     Z10, Z1, Z1
	VPANDD       Z11, Z0, Z2
	VPMULLW      Z12, Z2, Z2
	VPORD        Z1, Z2, Z0
	VPSUBUSB     Z13, Z0, Z1
	VPCMPGTB     Z0, Z14, K1
	VPMOVM2B     K1, Z2
	VPANDD       Z7, Z2, Z2
	VPORD        Z2, Z1, Z1
	VPSHUFB      Z1, Z15, Z1
	VPADDB       Z0, Z1, Z1
	VMOVDQU64    Z1, (DI)
	ADDQ         $48, SI
	ADDQ         $64, DI
	SUBQ         $48, BX
	JMP          e5loop

e5done:
	VZEROUPPER
	SUBQ BX, R10
	MOVQ R10, ret+24(FP)
	RET

// func _DecodeAVX512(dst *byte, src *byte, run *_Run) int
// Requires: AVX512F, AVX512BW. Handles 64 characters of src and 48 bytes of dst
// a step, while 64 characters remain and the room is 52 bytes (four stores of
// 16 bytes, 12 bytes apart). Returns how many characters it handled, or -1.
TEXT ·_DecodeAVX512(SB), NOSPLIT, $0-32
	MOVQ dst+0(FP), DI
	MOVQ src+8(FP), SI
	MOVQ run+16(FP), R11
	MOVQ (R11), BX
	MOVQ 8(R11), CX
	MOVQ 16(R11), R8
	MOVQ BX, R10
	TAB
	VBROADCASTI32X4 16(R8), Z8
	VBROADCASTI32X4 32(R8), Z9
	VBROADCASTI32X4 48(R8), Z10
	VPBROADCASTB 64(R8), Z11
	VPBROADCASTB 65(R8), Z12
	VQ64(0x0f0f0f0f0f0f0f0f, X13)
	VPBROADCASTQ X13, Z13
	VQ64(0x0140014001400140, X14)
	VPBROADCASTQ X14, Z14
	VQ64(0x0001100000011000, X15)
	VPBROADCASTQ X15, Z15
	VBROADCASTI32X4 b64pack<>(SB), Z7
	VPXORQ Z6, Z6, Z6

d5loop:
	CMPQ          BX, $64
	JB            d5done
	CMPQ          CX, $52
	JB            d5done
	VMOVDQU64     (SI), Z0
	VPSRLW        $4, Z0, Z1
	VPANDD        Z13, Z1, Z1
	VPANDD        Z13, Z0, Z2
	VPSHUFB       Z2, Z8, Z2
	VPSHUFB       Z1, Z9, Z3
	VPANDD        Z2, Z3, Z3
	VPORD         Z3, Z6, Z6
	VPCMPEQB      Z11, Z0, K1
	VPMOVM2B      K1, Z2
	VPANDD        Z12, Z2, Z2
	VPADDB        Z1, Z2, Z2
	VPSHUFB       Z2, Z10, Z2
	VPADDB        Z2, Z0, Z0
	VPMADDUBSW    Z14, Z0, Z0
	VPMADDWD      Z15, Z0, Z0
	VPSHUFB       Z7, Z0, Z0
	VMOVDQU       X0, (DI)
	VEXTRACTI32X4 $1, Z0, X1
	VMOVDQU       X1, 12(DI)
	VEXTRACTI32X4 $2, Z0, X2
	VMOVDQU       X2, 24(DI)
	VEXTRACTI32X4 $3, Z0, X3
	VMOVDQU       X3, 36(DI)
	ADDQ          $64, SI
	ADDQ          $48, DI
	SUBQ          $64, BX
	SUBQ          $48, CX
	JMP           d5loop

d5done:
	VPTESTMB Z6, Z6, K2
	KMOVQ    K2, AX
	TESTQ    AX, AX
	JNZ      d5bad
	VZEROUPPER
	SUBQ     BX, R10
	MOVQ     R10, ret+24(FP)
	RET

d5bad:
	VZEROUPPER
	MOVQ $-1, ret+24(FP)
	RET
