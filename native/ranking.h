#pragma once
#include "common/helpcode_utils.h"
#include "user_dictionary/user_dictionary_journal.h"
#include <algorithm>
#include <memory>
#include <nlohmann/json.hpp>
#include <sqlite3.h>
#include <string>

namespace backend_ranking {
using json = nlohmann::json;
using DB = std::unique_ptr<sqlite3, decltype(&sqlite3_close)>;
using Statement = std::unique_ptr<sqlite3_stmt, decltype(&sqlite3_finalize)>;
inline DB open(const std::string &path) {
  sqlite3 *p = nullptr;
  if (sqlite3_open_v2(path.c_str(), &p, SQLITE_OPEN_READWRITE, nullptr) !=
      SQLITE_OK) {
    if (p)
      sqlite3_close(p);
    return {nullptr, sqlite3_close};
  }
  return {p, sqlite3_close};
}
inline Statement prepare(sqlite3 *db, const char *sql) {
  sqlite3_stmt *p = nullptr;
  if (sqlite3_prepare_v2(db, sql, -1, &p, nullptr) != SQLITE_OK)
    return {nullptr, sqlite3_finalize};
  return {p, sqlite3_finalize};
}
inline void bind_text(sqlite3_stmt *s, int index, const std::string &value) {
  sqlite3_bind_text(s, index, value.c_str(), -1, SQLITE_TRANSIENT);
}
// State transport for the pinned Engine journal. Selection arithmetic and
// weight changes remain in Engine.
inline bool restore_counter(const std::string &path, const json &value) {
  auto db = open(path);
  if (!db)
    return false;
  auto stmt = prepare(
      db.get(),
      "INSERT INTO "
      "candidate_selection_state(context_key,entry_key,value,selection_count) "
      "VALUES(?1,?2,?3,?4) ON CONFLICT(context_key,entry_key,value) DO UPDATE "
      "SET selection_count=excluded.selection_count");
  if (!stmt)
    return false;
  bind_text(stmt.get(), 1, value.at("context"));
  bind_text(stmt.get(), 2, value.at("code"));
  bind_text(stmt.get(), 3, value.at("word"));
  sqlite3_bind_int(stmt.get(), 4, value.at("count"));
  return sqlite3_step(stmt.get()) == SQLITE_DONE;
}
inline json apply(const json &action, bool remove, const std::string &context,
                  const std::vector<WordItem> &candidates,
                  const std::string &kind, const std::string &dictionary,
                  const std::string &journal) {
  const auto key = action.at("code").get<std::string>(),
             word = action.at("word").get<std::string>();
  const auto selected = std::find_if(
      candidates.begin(), candidates.end(), [&](const WordItem &item) {
        const auto code =
            item.canonical_pinyin.empty() ? item.pinyin : item.canonical_pinyin;
        return code == key && item.word == word;
      });
  if (selected == candidates.end() || context.empty())
    return {{"error", "invalid_request"}};
  if (remove) {
    if (kind != "english" && HelpcodeUtils::count_utf8_chars(word) <= 1)
      return {{"error", "invalid_request"}};
    const auto type =
        kind == "english" ? user_dictionary::DictionaryKind::English
        : kind == "wubi"  ? user_dictionary::DictionaryKind::Wubi
                          : user_dictionary::DictionaryKind::Pinyin;
    const bool inserted =
        user_dictionary::is_user_inserted(journal, type, key, word);
    if (!user_dictionary::delete_dictionary_candidate(dictionary, journal, type,
                                                      key, word))
      return {{"error", "engine_failure"}};
    return {{"deleted",
             {{"kind", kind},
              {"code", key},
              {"word", word},
              {"weight", selected->weight},
              {"user_inserted", inserted}}},
            {"changed", true}};
  }
  bool changed = false;
  const auto mode = action.value("mode", std::string("pin"));
  const int step = action.value("linear_step", 1),
            trigger = action.value("trigger_count", 1);
  const bool force = action.value("force_top", false);
  if ((mode != "disabled" && mode != "pin" && mode != "halve" &&
       mode != "linear" && mode != "promote") ||
      step < 1 || step > 100 || trigger < 1 || trigger > 10)
    return {{"error", "invalid_request"}};
  const bool success =
      kind == "english"
          ? user_dictionary::adjust_english_candidate_ranking(
                dictionary, journal, context, candidates, key, word, mode, step,
                trigger, force, &changed)
          : user_dictionary::adjust_candidate_ranking(
                dictionary, journal, context, candidates, key, word, mode, step,
                trigger, force, &changed,
                kind == "wubi" ? user_dictionary::DictionaryKind::Wubi
                               : user_dictionary::DictionaryKind::Pinyin);
  if (!success)
    return {{"error", "invalid_request"}};
  auto db = open(journal);
  if (!db)
    return {{"error", "engine_failure"}};
  auto counter =
      prepare(db.get(), "SELECT selection_count FROM candidate_selection_state "
                        "WHERE context_key=?1 AND entry_key=?2 AND value=?3");
  if (!counter)
    return {{"error", "engine_failure"}};
  bind_text(counter.get(), 1, context);
  bind_text(counter.get(), 2, key);
  bind_text(counter.get(), 3, word);
  int count = 0;
  int status = sqlite3_step(counter.get());
  if (status == SQLITE_ROW)
    count = sqlite3_column_int(counter.get(), 0);
  else if (status != SQLITE_DONE)
    return {{"error", "engine_failure"}};
  counter.reset();
  auto updates = json::array();
  auto weight = prepare(
      db.get(), "SELECT weight FROM user_dictionary_operations WHERE "
                "dictionary=?1 AND key=?2 AND value=?3 AND operation='upsert'");
  if (!weight)
    return {{"error", "engine_failure"}};
  for (const auto &item : candidates) {
    const auto code =
        item.canonical_pinyin.empty() ? item.pinyin : item.canonical_pinyin;
    sqlite3_reset(weight.get());
    sqlite3_clear_bindings(weight.get());
    bind_text(weight.get(), 1, kind);
    bind_text(weight.get(), 2, code);
    bind_text(weight.get(), 3, item.word);
    status = sqlite3_step(weight.get());
    if (status == SQLITE_DONE)
      continue;
    if (status != SQLITE_ROW)
      return {{"error", "engine_failure"}};
    const auto updated = sqlite3_column_int64(weight.get(), 0);
    sqlite3_reset(weight.get());
    if (updated != item.weight) {
      const auto type =
          kind == "english" ? user_dictionary::DictionaryKind::English
          : kind == "wubi"  ? user_dictionary::DictionaryKind::Wubi
                            : user_dictionary::DictionaryKind::Pinyin;
      updates.push_back(
          {{"kind", kind},
           {"code", code},
           {"word", item.word},
           {"weight", updated},
           {"user_inserted", user_dictionary::is_user_inserted(
                                 journal, type, code, item.word)}});
    }
  }
  return {
      {"updates", updates},
      {"selection",
       {{"context", context}, {"code", key}, {"word", word}, {"count", count}}},
      {"changed", changed}};
}
} // namespace backend_ranking
