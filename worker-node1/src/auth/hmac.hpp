#pragma once
#include <string>
#include <openssl/hmac.h>
#include <openssl/evp.h>
#include <ctime>
#include <stdexcept>
#include <sstream>
#include <vector>
#include <cstring>

namespace auth {

// base64url_decode decodes a URL-safe base64 string (no padding).
inline std::vector<uint8_t> base64url_decode(const std::string& s) {
    std::string b = s;
    for (char& c : b) {
        if (c == '-') c = '+';
        else if (c == '_') c = '/';
    }
    // Add padding
    while (b.size() % 4) b += '=';
    std::vector<uint8_t> out(b.size());
    int len = EVP_DecodeBlock(out.data(),
                              reinterpret_cast<const uint8_t*>(b.c_str()), b.size());
    if (len < 0) throw std::runtime_error("base64 decode failed");
    // Remove padding bytes
    int pad = 0;
    for (int i = b.size()-1; i >= 0 && b[i] == '='; --i) pad++;
    out.resize(len - pad);
    return out;
}

// TokenValidator validates X-Master-Token values using HMAC-SHA256.
class TokenValidator {
public:
    TokenValidator(const std::string& secret, int ttl_seconds)
        : secret_(secret), ttl_(ttl_seconds) {}

    // validate returns true if token is well-formed, not expired, and MAC is valid.
    bool validate(const std::string& token) const {
        auto dot = token.find('.');
        if (dot == std::string::npos) return false;

        std::string ts_str = token.substr(0, dot);
        std::string mac_b64 = token.substr(dot + 1);

        long ts = 0;
        try { ts = std::stol(ts_str); } catch (...) { return false; }

        time_t now = std::time(nullptr);
        if (now - ts > ttl_ || ts - now > 5) return false;

        // Compute expected HMAC
        uint8_t expected[32];
        unsigned int expected_len = 0;
        HMAC(EVP_sha256(),
             secret_.c_str(), secret_.size(),
             reinterpret_cast<const uint8_t*>(ts_str.c_str()), ts_str.size(),
             expected, &expected_len);

        // Decode received MAC
        std::vector<uint8_t> got;
        try { got = base64url_decode(mac_b64); } catch (...) { return false; }

        if (got.size() != expected_len) return false;
        // Constant-time comparison
        int diff = 0;
        for (unsigned int i = 0; i < expected_len; ++i)
            diff |= expected[i] ^ got[i];
        return diff == 0;
    }

private:
    std::string secret_;
    int ttl_;
};

} // namespace auth