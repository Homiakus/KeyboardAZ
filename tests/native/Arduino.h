#pragma once

#include <cstddef>
#include <cstdint>
#include <deque>
#include <string>

#define INPUT_PULLUP 0x2
#define LOW 0
#define HIGH 1

class HardwareSerial {
public:
    void begin(uint32_t) {}
    std::size_t write(const uint8_t* data, std::size_t size) {
        output.append(reinterpret_cast<const char*>(data), size);
        return size;
    }
    int available() const { return static_cast<int>(input.size()); }
    int read() {
        if (input.empty()) return -1;
        const int value = input.front();
        input.pop_front();
        return value;
    }
    void feed(const std::string& text) {
        for (unsigned char ch : text) input.push_back(ch);
    }
    std::string takeOutput() {
        std::string result = output;
        output.clear();
        return result;
    }

    std::string output;
    std::deque<int> input;
};

extern HardwareSerial Serial;

void pinMode(uint8_t pin, uint8_t mode);
int digitalRead(uint8_t pin);
uint32_t millis();
uint32_t micros();
void delay(uint32_t ms);
void delayMicroseconds(uint32_t us);
