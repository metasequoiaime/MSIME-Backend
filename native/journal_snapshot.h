#pragma once
#include "ranking.h"

namespace backend_ranking {
// Transport a complete, already-resolved snapshot into the pinned Engine
// journal in one transaction. Dictionary partitioning, replay and ranking
// remain in Engine. Unlike incremental journal operations, these rows contain
// final ownership/state.
class SnapshotWriter {
  DB db_{nullptr, sqlite3_close};
  Statement entry_{nullptr, sqlite3_finalize};
  Statement fixed_{nullptr, sqlite3_finalize};
  Statement selection_{nullptr, sqlite3_finalize};
  bool active_ = false;

public:
  explicit SnapshotWriter(const std::string &path) : db_(open(path)) {
    if (!db_ || sqlite3_exec(db_.get(), "BEGIN IMMEDIATE", nullptr, nullptr,
                             nullptr) != SQLITE_OK)
      return;
    active_ = true;
    entry_ =
        prepare(db_.get(),
                "INSERT INTO "
                "user_dictionary_operations(dictionary,key,value,operation,"
                "weight,display,user_inserted) VALUES(?1,?2,?3,?4,?5,?6,?7) ON "
                "CONFLICT(dictionary,key,value) DO UPDATE SET "
                "operation=excluded.operation,weight=excluded.weight,display="
                "excluded.display,user_inserted=excluded.user_inserted");
    fixed_ = prepare(db_.get(), "INSERT INTO "
                                "fixed_candidate_positions(context_key,entry_"
                                "key,value,position) VALUES(?1,?2,?3,?4)");
    selection_ =
        prepare(db_.get(), "INSERT INTO "
                           "candidate_selection_state(context_key,entry_key,"
                           "value,selection_count) VALUES(?1,?2,?3,?4)");
  }
  ~SnapshotWriter() {
    if (active_)
      sqlite3_exec(db_.get(), "ROLLBACK", nullptr, nullptr, nullptr);
  }
  bool ready() const { return active_ && entry_ && fixed_ && selection_; }
  bool entry(const json &value, bool deleted) {
    if (value.is_null())
      return true;
    const auto kind = value.at("kind").get<std::string>();
    if (kind != "pinyin" && kind != "wubi" && kind != "english" &&
        kind != "quick")
      return false;
    sqlite3_reset(entry_.get());
    sqlite3_clear_bindings(entry_.get());
    bind_text(entry_.get(), 1, kind);
    bind_text(entry_.get(), 2, value.at("code"));
    bind_text(entry_.get(), 3, value.at("word"));
    bind_text(entry_.get(), 4, deleted ? "delete" : "upsert");
    sqlite3_bind_int64(entry_.get(), 5,
                       deleted ? 0 : value.at("weight").get<std::int64_t>());
    bind_text(entry_.get(), 6,
              !deleted && kind == "english"
                  ? value.at("word").get<std::string>()
                  : std::string());
    sqlite3_bind_int(entry_.get(), 7,
                     !deleted && value.value("user_inserted", true));
    return sqlite3_step(entry_.get()) == SQLITE_DONE;
  }
  bool position(const json &value) {
    return keyed(fixed_.get(), value, value.at("position").get<int>());
  }
  bool selection(const json &value) {
    return keyed(selection_.get(), value, value.at("count").get<int>());
  }
  bool commit() {
    entry_.reset();
    fixed_.reset();
    selection_.reset();
    if (!active_ || sqlite3_exec(db_.get(), "COMMIT", nullptr, nullptr,
                                 nullptr) != SQLITE_OK)
      return false;
    active_ = false;
    db_.reset();
    return true;
  }

private:
  bool keyed(sqlite3_stmt *statement, const json &value, int number) {
    sqlite3_reset(statement);
    sqlite3_clear_bindings(statement);
    bind_text(statement, 1, value.at("context"));
    bind_text(statement, 2, value.at("code"));
    bind_text(statement, 3, value.at("word"));
    sqlite3_bind_int(statement, 4, number);
    return sqlite3_step(statement) == SQLITE_DONE;
  }
};
} // namespace backend_ranking
