/**
 * 数据页的交互逻辑。
 *
 * 分页是必须的：7792 条断言一次性渲染会让界面卡住，而卡住的界面
 * 会让人以为「数据坏了」。翻页时**保留筛选条件**，否则人会以为筛选失效。
 */
import { useCallback, useEffect } from "react";
import { toast } from "sonner";

import {
  Assertions,
  Quality,
  Schema,
  SubjectDetail,
  Subjects,
} from "../../../../bindings/github.com/ngnl5/ssot/internal/api/dataservice";
import type { FilterInput } from "../../../../bindings/github.com/ngnl5/ssot/internal/api/models";
import { PAGE_SIZE, useDataStore } from "./store";

export function useDataExplorer() {
  const s = useDataStore();

  const filter = useCallback(
    (): FilterInput =>
      ({
        entity: s.entity,
        status: s.status,
        predicate: s.predicate.trim(),
        confidence: s.confidence,
        artifact: "",
        revision: "",
        subject: "",
        limit: 0,
      }) as FilterInput,
    [s.entity, s.status, s.predicate, s.confidence],
  );

  const reload = useCallback(
    async (offset = 0) => {
      s.setLoading(true);
      s.setError(null);
      try {
        const [page, quality, schemas] = await Promise.all([
          Assertions(filter(), PAGE_SIZE, offset),
          Quality(),
          Schema(),
        ]);
        s.setPage(page);
        s.setQuality(quality ?? []);
        s.setSchemas(schemas ?? []);
      } catch (e) {
        s.setError(String(e));
      } finally {
        s.setLoading(false);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [filter],
  );

  useEffect(() => {
    void reload(0);
  }, [reload]);

  const openSubject = useCallback(
    async (entity: string, subject: string) => {
      s.setSelected({ entity, subject });
      try {
        s.setDetail((await SubjectDetail(entity, subject)) ?? []);
      } catch (e) {
        s.setDetail([]);
        toast.error(String(e));
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  const subjects = useCallback(async (entity: string): Promise<string[]> => {
    try {
      return (await Subjects(entity)) ?? [];
    } catch {
      return [];
    }
  }, []);

  return { ...s, reload, filter, openSubject, subjects, pageSize: PAGE_SIZE };
}
