# codec converts between bytes, hex, bits, integers and the base encodings.
# Anything that is conceptually bytes accepts a str as well.
#
# Two naming styles sit side by side, and the reason is readable here. The
# twelve conversions of the brief are x2y - bytes2hex, int2bits. The base
# encodings are encode and decode, because a name ending in a digit cannot take
# the 2 infix: base642bytes reads as "base 642 bytes".
#
# Two spellings of hex are here too, and both were asked for: bytes2hex is
# lowercase and unpadded, because it is bytes; bits2hex is uppercase and pads
# to a whole digit, because it is a number.

def main():
    print("hex of bytes", bytes2hex(b"\x00\xffAB"))
    print("bytes of hex", hex2bytes("4142"))
    print("bits of a byte", bytes2bits("A"))
    print("bytes of bits", bits2bytes("0100000101000010"))
    print("int of bytes", bytes2int(b"\x01\x00"))
    print("bytes of int, as hex", bytes2hex(int2bytes(256, 4)))
    print("hex of bits", bits2hex("11010"))
    print("bits of hex", hex2bits("0xAF"))
    print("bits of int", int2bits(5, 8))
    print("int of bits", bits2int("100101100"))
    print("hex of int", int2hex(300, 4))
    print("int of hex", hex2int("12C"))

    # The base encodings. The URL-safe alphabet is unpadded, which is what a
    # JSON Web Token carries; both decoders take padded or unpadded text.
    print("base64", base64.encode("foobar"))
    print("base64 back", base64.decode("Zm9vYmFy"))
    print("base64url", base64.urlencode(b"\xfb\xff\xbf"))
    print("standard, for contrast", base64.encode(b"\xfb\xff\xbf"))
    print("base32", base32.encode("foobar"))

    # A checksum, and the same checksum taken in two parts.
    print("crc32", crc32("123456789"))
    print("crc32 in two parts", crc32("56789", crc32("1234")))
