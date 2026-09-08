#include "online/AiSuggestionProvider.h"
#include "online/TranslationProvider.h"
#include "VoiceInput.h"
#include "cloud/custom_translation.h"
#include "voice-input/voice_batch_protocol.h"
#include "voice-input/voice_providers.h"
#include <msime/voice/wav_writer.h>
#include <msime/voice/cloud_stt_worker.h>
#include <msime/voice/text_polisher.h>
#include <iostream>
#include <stdexcept>

void require(bool result, const char *feature) {
    if (!result) throw std::runtime_error(feature);
    std::cout << feature << " passed\n";
}
int main(int argc, char **argv) {
    if (argc != 3) return 2;
    try {
        const std::string base = argv[1], token = argv[2];
        namespace online = metasequoia::linux_ime::online;
        namespace voice = metasequoia::voice;
        auto transport = std::make_shared<online::CurlHttpTransport>();
        const auto cancelled = [] { return false; };
        metasequoia::OnlineQuery query;
        query.query_text = "ni'hao"; query.pinyin_segments = {"ni", "hao"}; query.ai_eligible = true;
        online::AiSuggestionConfig ai;
        ai.enabled = true; ai.provider = online::AiProvider::Custom; ai.endpoint = base + "/v1/chat/completions";
        ai.token = token; ai.model = "client-model";
        online::AiSuggestionProvider suggestions(transport);
        require(suggestions.fetch(query, "", ai, cancelled).value_or("") == "你好", "Linux AI");
        online::TranslationProvider translation(std::string(), transport);
        require(translation.lookup("测试", "en", base + "/v1/translate", token, cancelled).value_or("") == "test", "Linux translation");
        const auto windowsTranslation = CustomTranslation::TextTranslateBatch({base + "/v1/translate", token}, {"测试"}, "AUTO", "EN");
        require(windowsTranslation.size() == 1 && windowsTranslation[0] == "test", "Windows translation");
        const std::vector<float> samples(1600, 0.0f);
        const auto wav = voice::WavWriter::create_wav(samples);
        metasequoia::linux_ime::VoiceInputConfig config;
        config.enabled = true; config.endpoint = base + "/v1/audio/transcriptions"; config.token = token;
        config.polish_enabled = true; config.polish_endpoint = base + "/v1/chat/completions";
        metasequoia::linux_ime::VoiceInputProvider linuxVoice(transport);
        require(linuxVoice.transcribe(std::string_view(reinterpret_cast<const char *>(wav.data()), wav.size()), config, cancelled).value_or("") == "测试", "Linux transcription");
        require(linuxVoice.polish("测试", config, cancelled).value_or("") == "整理结果", "Linux polish");
        voice::RequestOptions sharedOptions{};
        sharedOptions.endpoint = base + "/v1/audio/transcriptions"; sharedOptions.model = "client-model"; sharedOptions.token = token;
        voice::CloudSttWorker sharedVoice(sharedOptions);
        require(sharedVoice.recognize(samples) == "测试", "Shared voice transcription");
        sharedOptions.endpoint = base + "/v1/chat/completions";
        voice::TextPolisher sharedPolish(sharedOptions, "Return cleaned text.");
        require(sharedPolish.polish("测试") == "整理结果", "Shared voice polish");
        ::VoiceInputConfig windowsVoice;
        windowsVoice.asr_provider = "msime-stream";
        windowsVoice.asr_tokens["doubao"] = "synthetic-vendor-token";
        require(VoiceInput::ResolveAsrToken(windowsVoice).empty(), "Streaming device token isolated from vendor token");
        windowsVoice.asr_tokens["msime-stream"] = token;
        require(VoiceInput::IsDoubaoAsrProvider(windowsVoice.asr_provider) &&
                VoiceInput::ResolveAsrToken(windowsVoice) == token &&
                VoiceInput::DefaultAsrEndpoint(windowsVoice.asr_provider).empty(), "Streaming backend provider selection");
        windowsVoice.asr_provider = "openai"; windowsVoice.asr_model = "client-model";
        const auto multipart = VoiceInput::BuildBatchTranscription(samples, windowsVoice);
        online::HttpRequest request;
        request.method = online::HttpMethod::Post; request.url = base + "/v1/audio/transcriptions";
        request.headers = {"Authorization: Bearer " + token, "Content-Type: " + multipart.content_type};
        request.body = multipart.body;
        const auto response = transport->perform(request, cancelled);
        require(response.status_code == 200 && voice::parse_transcription(response.body) == "测试", "Windows batch transcription protocol");
        windowsVoice.polish_provider = "siliconflow";
        request.url = base + "/v1/chat/completions";
        request.headers = {"Authorization: Bearer " + token, "Content-Type: application/json"};
        request.body = VoiceInput::BuildBatchPolish("测试", windowsVoice);
        const auto polish = transport->perform(request, cancelled);
        require(polish.status_code == 200 && voice::parse_polished_text(polish.body) == "整理结果", "Windows batch polish protocol");
        config.token = "wrong-token";
        require(!linuxVoice.transcribe(std::string_view(reinterpret_cast<const char *>(wav.data()), wav.size()), config, cancelled), "Rejected transcription credential");
    } catch (const std::exception &error) {
        std::cerr << error.what() << '\n'; return 1;
    }
    return 0;
}
