cmake_minimum_required(VERSION 3.25)
project(MSIMEBackendNativeIntegration LANGUAGES C CXX)
set(CMAKE_CXX_STANDARD 17)
set(BUILD_TESTING OFF CACHE BOOL "" FORCE)
set(METASEQUOIA_IME_BUILD_IBUS OFF CACHE BOOL "" FORCE)
add_subdirectory(/linux linux)
add_executable(native-services ../native_services_client.cpp
    /windows/server/src/cloud/custom_translation.cpp
    /windows/server/src/voice-input/voice_batch_protocol.cpp
    /windows/server/src/voice-input/voice_providers.cpp)
target_include_directories(native-services PRIVATE /windows/server/src /linux/vendor)
target_link_libraries(native-services PRIVATE MetasequoiaImeLinuxOnline MetasequoiaImeLinuxVoice MetasequoiaIme::Voice)
target_compile_options(native-services PRIVATE -Wall -Wextra -Werror)
