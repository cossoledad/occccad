#pragma once

namespace Base {
class ConsoleObserver {
public:
    template <typename... Args>
    void Log(const char*, Args&&...) const noexcept {}

    template <typename... Args>
    void Warning(const char*, Args&&...) const noexcept {}

    template <typename... Args>
    void Error(const char*, Args&&...) const noexcept {}
};

inline ConsoleObserver& Console() noexcept {
    static ConsoleObserver console;
    return console;
}
}  // namespace Base
