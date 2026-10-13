import test from 'node:test';
import assert from 'node:assert/strict';
import { tasks, filterTasks } from '../src/task-data.js';

test('fallback task catalog contains unique identifiers', () => {
  const ids = tasks.map((task) => task.id);
  assert.equal(new Set(ids).size, ids.length);
});

test('empty search returns the complete fallback catalog', () => {
  assert.deepEqual(filterTasks(tasks, ''), tasks);
  assert.deepEqual(filterTasks(tasks, '   '), tasks);
});

test('search matches task title case-insensitively', () => {
  assert.equal(filterTasks(tasks, 'СУММА').length, 1);
  assert.equal(filterTasks(tasks, 'сумма')[0].id, 'A-104');
});

test('search matches category and task id', () => {
  assert.equal(filterTasks(tasks, 'Графы')[0].id, 'B-218');
  assert.equal(filterTasks(tasks, 'C-039')[0].title, 'Последний рубеж');
});

test('unknown search returns no results', () => {
  assert.deepEqual(filterTasks(tasks, 'задача которой нет'), []);
});
