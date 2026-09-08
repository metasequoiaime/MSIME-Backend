#include <nlohmann/json.hpp>
#include "catalog.h"
#include <SimpleConverter.hpp>
#include <cpp-pinyin/Pinyin.h>
#include <cpp-pinyin/G2pglobal.h>
#include "contracts/assets/assets.h"
#include "local_modes/unicode_query.h"
#include "local_modes/date_time_query.h"
#include "local_modes/emoji_query.h"
#include "local_modes/kaomoji_query.h"
#include "local_modes/jianpin_query.h"
#include "english/english_dictionary.h"
#include "include/metasequoia/personal_dictionary.h"
#include "common/helpcode_utils.h"
#include "quanpin/quanpin_query.h"
#include "schemes/quanpin_scheme.h"
#include "schemes/shuangpin_scheme.h"
#include "schemes/wubi_scheme.h"
#include "providers/pinyin_candidate_provider.h"
#include "providers/wubi_candidate_provider.h"
#include "local_modes/quick_phrase_query.h"
#include <filesystem>
#include <iostream>
#include <stdexcept>

using json = nlohmann::json;
using namespace metasequoia;

static json candidates(const std::vector<WordItem>& items) {
    auto out = json::array();
    for (const auto& item : items) {
        out.push_back({{"code", item.pinyin}, {"canonical_pinyin", item.canonical_pinyin},
                       {"word", item.word}, {"weight", item.weight}, {"fixed_position", item.fixed_position}});
    }
    return {{"candidates", out}};
}

// One bounded request per process: mutable Engine globals never cross users or requests.
// Paths come only from the host process argument, never from the JSON request.
static json execute(const json& request, const std::filesystem::path& resources, const std::filesystem::path& scratch) {
    const auto op = request.at("operation").get<std::string>();
    const auto text = request.value("text", std::string());
    const int limit = request.value("limit", 20);
    if (limit < 1 || limit > 200 || text.size() > 8192) throw std::invalid_argument("invalid_request");
    if (op == "annotate_batch") {
        const auto& words = request.at("words");
        if (!words.is_array() || words.empty() || words.size() > 50) throw std::invalid_argument("invalid_request");
        auto entries = json::array();
        for (const auto& word : words) {
            auto result = execute({{"operation", "annotate"}, {"text", word}}, resources, scratch);
            if (result.contains("error")) return result;
            entries.push_back(result);
        }
        return {{"entries", entries}};
    }
    if (op == "validate_dictionary_batch") {
        const auto& entries = request.at("entries");
        if (!entries.is_array() || entries.empty() || entries.size() > 50) throw std::invalid_argument("invalid_request");
        auto validated = json::array();
        for (auto entry : entries) {
            entry["operation"] = "validate_dictionary";
            const auto result = execute(entry, resources, scratch);
            if (result.contains("error")) return result;
            validated.push_back(result);
        }
        return {{"entries", validated}};
    }
    if (op == "unicode") return candidates(local_modes::query_unicode(text, limit));
    if (op == "datetime") {
        const auto& date = request.at("date");
        local_modes::LocalDateTime now{date.at("year"), date.at("month"), date.at("day"),
                                       date.at("weekday"), date.at("hour"), date.at("minute"), date.at("second")};
        if (now.year < 1 || now.year > 9999 || now.month < 1 || now.month > 12 || now.day < 1 || now.day > 31 ||
            now.weekday > 6 || now.hour > 23 || now.minute > 59 || now.second > 59 ||
            !local_modes::is_date_time_keyword(text)) throw std::invalid_argument("invalid_request");
        return candidates(local_modes::query_date_time(text, &now, limit));
    }
    if (op == "validate_dictionary") {
        const auto kind = request.at("kind").get<std::string>();
        PersonalDictionaryKind type;
        if (kind == "pinyin") type = PersonalDictionaryKind::Pinyin;
        else if (kind == "wubi") type = PersonalDictionaryKind::Wubi;
        else if (kind == "quick") type = PersonalDictionaryKind::QuickPhrase;
        else if (kind == "english") type = PersonalDictionaryKind::English;
        else throw std::invalid_argument("invalid_request");
        auto result = validate_personal_dictionary_entry({type, request.at("code"), text, request.value("weight", 100000LL)});
        if (!result.entry) return {{"error", "invalid_dictionary_entry"}};
        return {{"kind", kind}, {"code", result.entry->key}, {"word", result.entry->value}, {"weight", result.entry->weight}};
    }
    const auto scheme_name = request.value("scheme", std::string("pinyin"));
    SchemeType scheme;
    if (scheme_name == "pinyin") scheme = SchemeType::Quanpin;
    else if (scheme_name == "shuangpin") scheme = SchemeType::Shuangpin;
    else if (scheme_name == "wubi") scheme = SchemeType::Wubi;
    else throw std::invalid_argument("invalid_request");
    const auto& profile = GetShuangpinProfile(request.value("profile", std::string("xiaohe")));
    QueryRequest query;
    if (op == "candidates" || op == "segmentation") {
        if (scheme == SchemeType::Quanpin) {
            QuanpinScheme input; input.set_raw_input(text, text); query = input.build_request();
        } else if (scheme == SchemeType::Shuangpin) {
            ShuangpinScheme input(profile); input.set_raw_input(text, text); query = input.build_request();
        } else {
            WubiScheme input; input.set_raw_input(text, text); query = input.build_request();
        }
        if (!query.valid) throw std::invalid_argument("invalid_request");
        if (op == "segmentation") return {{"raw", query.raw_segmentation}, {"normalized", query.normalized_segmentation}};
    }
    if (resources.empty() || !resources.is_absolute()) return {{"error", "resources_unavailable"}};
    if (op == "annotate") {
        const auto dictionary = resources / "pinyin";
        if (!std::filesystem::is_directory(dictionary)) return {{"error", "resources_unavailable"}};
        static std::unique_ptr<Pinyin::Pinyin> annotator;
        if (!annotator) {Pinyin::setDictionaryPath(dictionary); annotator = std::make_unique<Pinyin::Pinyin>();}
        if (!annotator->initialized()) return {{"error", "resources_unavailable"}};
        const auto result = annotator->hanziToPinyin(text, Pinyin::ManTone::Style::NORMAL, Pinyin::Error::Default, false, false, false);
        std::string code;
        for (const auto& item : result) {
            if (item.error || item.pinyin.empty()) return {{"error", "invalid_request"}};
            auto syllable = item.pinyin;
            for (std::size_t pos = 0; (pos = syllable.find("ü", pos)) != std::string::npos;) syllable.replace(pos, 2, "v");
            if (!code.empty()) code += "'";
            code += syllable;
        }
        const auto validated = validate_personal_dictionary_entry({PersonalDictionaryKind::Pinyin, code, text, 10});
        if (!validated.entry) return {{"error", "invalid_dictionary_entry"}};
        return {{"code", validated.entry->key}, {"word", text}};
    }
    if (op == "convert") {
        const auto config = resources / "opencc" / "s2t.json";
        if (!std::filesystem::is_regular_file(config)) return {{"error", "resources_unavailable"}};
        opencc::SimpleConverter converter(config.string());
        return {{"text", converter.Convert(text)}, {"conversion", "s2t"}};
    }
    if (op == "catalog") return query_catalog(request, resources);
    if (op == "helpcode") {
        const auto schema = request.value("schema", std::string("lantian"));
        if (!HelpcodeUtils::is_supported_helpcode_schema(schema)) throw std::invalid_argument("invalid_request");
        const auto keymap = HelpcodeUtils::load_helpcode_keymap(resources, schema);
        if (!keymap || keymap->empty()) return {{"error", "resources_unavailable"}};
        return {{"text", HelpcodeUtils::compute_helpcodes(text, false, keymap.get())}, {"schema", schema}};
    }
    if (op == "emoji" || op == "kaomoji" || op == "jianpin" || op == "quick") {
        auto path = resources / ((op == "jianpin" || op == "quick") ? assets::main_dictionary : assets::other_dictionary);
        if (!std::filesystem::is_regular_file(path)) return {{"error", "resources_unavailable"}};
        local_modes::LocalQueryResult result;
        if (op == "emoji") result = local_modes::query_emoji(text, scheme, path, limit, profile);
        else if (op == "kaomoji") result = local_modes::query_kaomoji(text, scheme, path, limit, profile);
        else if (op == "jianpin") result = local_modes::query_jianpin(text, scheme, path, limit, profile);
        else result = local_modes::query_quick_phrases(text, path, limit);
        if (result.diagnostic) return {{"error", "resources_unavailable"}};
        return candidates(result.candidates);
    }
    if (op == "candidates") {
        auto path = resources / assets::main_dictionary;
        if (!std::filesystem::is_regular_file(path) || scratch.empty() || !scratch.is_absolute())
            return {{"error", "resources_unavailable"}};
        std::vector<WordItem> items;
        if (scheme == SchemeType::Wubi) {
            WubiCandidateProvider provider(path.string()); items = provider.query(query);
        } else {
            RuntimePaths paths{resources, scratch, scratch, resources};
            PinyinCandidateProvider provider(profile, paths); items = provider.query(query);
        }
        if (items.size() > static_cast<std::size_t>(limit)) items.resize(limit);
        auto result = candidates(items);
        result["raw_segmentation"] = query.raw_segmentation;
        result["normalized_segmentation"] = query.normalized_segmentation;
        return result;
    }
    if (op == "english" || op == "gloss") {
        auto path = resources / "english.db";
        if (!std::filesystem::is_regular_file(path)) return {{"error", "resources_unavailable"}};
        EnglishDictionary dictionary(path.string(), false);
        if (!dictionary.ready()) return {{"error", "resources_unavailable"}};
        if (op == "english") return candidates(dictionary.query_prefix(text, limit));
        const auto direction = request.value("direction", std::string("en-zh"));
        if (direction != "en-zh" && direction != "zh-en") throw std::invalid_argument("invalid_request");
        return {{"text", direction == "en-zh" ? dictionary.query_chinese_gloss(text) : dictionary.query_english_gloss(text)}};
    }
    return {{"error", "unknown_operation"}};
}

int main(int argc, char** argv) {
    try {
        std::string raw;
        char c;
        while (std::cin.get(c)) {
            if (raw.size() >= 65536) throw std::invalid_argument("invalid_request");
            raw.push_back(c);
        }
        auto response = execute(json::parse(raw), argc >= 2 ? std::filesystem::u8path(argv[1]) : std::filesystem::path(),
                                argc == 3 ? std::filesystem::u8path(argv[2]) : std::filesystem::path());
        std::cout << response.dump() << '\n';
    } catch (const json::exception&) {
        std::cout << "{\"error\":\"invalid_request\"}\n";
    } catch (const std::invalid_argument&) {
        std::cout << "{\"error\":\"invalid_request\"}\n";
    } catch (...) {
        // Native diagnostics may contain paths/input: never forward them to HTTP clients or logs.
        std::cout << "{\"error\":\"engine_failure\"}\n";
    }
}
