# The conversions between bytes, hex, bits and integers, the base encodings,
# and a checksum. Anything that is conceptually bytes accepts a str as well.
#
# Each form is a module named for it: hex, binary and integer convert to and
# from bytes, and base64 and base32 encode and decode. The integer module is not
# called int or bytes, because those already name Starlark's own builtins.
#
# Hex is written in upper case and read in either, with or without 0x; bits are
# read with or without 0b. A width counts what it writes: hex digits for hex,
# bits for binary, bytes for integer.

def main():
    print("hex of bytes", hex.from_bytes(b"\x00\xffAB"))
    print("bytes of hex", hex.to_bytes("4142"))
    print("hex of int", hex.from_int(300, 4))
    print("int of hex", hex.to_int("12c"))
    print("hex of bits", hex.from_binary("11010"))
    print("bits of hex", hex.to_binary("0xAF"))
    print("bits of a byte", binary.from_bytes("A"))
    print("bytes of bits", binary.to_bytes("0100000101000010"))
    print("bits of int", binary.from_int(5, 8))
    print("int of bits", binary.to_int("100101100"))
    print("int of bytes", integer.from_bytes(b"\x01\x00"))
    print("bytes of int, as hex", hex.from_bytes(integer.to_bytes(256, 4)))
    print("little-endian", hex.from_bytes(integer.to_bytes(256, 4, "little")))
    print("signed", hex.from_bytes(integer.to_bytes(-2, 2, signed = True)))

    # A packet written a piece at a time, and taken apart again in order: a
    # magic number, a length, a body, and a signed little-endian trailer. The
    # methods are named as Go names them.
    packet = buf.new()
    packet.write_bytes(hex.to_bytes("CAFE"))
    packet.write_uint16(3)
    packet.write_string("abc")
    packet.write_int16(-1, "little")
    print("packet", hex.from_bytes(packet.bytes()))

    reader = buf.reader(packet.bytes())
    print("magic", hex.from_bytes(reader.read_bytes(2)))
    print("body", reader.read_string(reader.read_uint16()))
    print("trailer", reader.read_int16("little"), "left", len(reader))

    # The base encodings. The URL-safe alphabet is unpadded, which is what a
    # JSON Web Token carries; both decoders take padded or unpadded text.
    print("base64", base64.encode("foobar"))
    print("base64 back", base64.decode("Zm9vYmFy"))
    print("base64url", base64.urlencode(b"\xfb\xff\xbf"))
    print("standard, for contrast", base64.encode(b"\xfb\xff\xbf"))
    print("base32", base32.encode("foobar"))

    # A checksum, and the same checksum taken in two parts.
    print("crc32", hash.crc32("123456789"))
    print("crc32 in two parts", hash.crc32("56789", hash.crc32("1234")))
