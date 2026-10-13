export const tasks = [
  { id: 'A-104', title: 'Сумма на отрезке', category: 'Алгоритмы', level: 'Лёгкая', time: '15 мин', solved: '2.4k', color: 'violet' },
  { id: 'B-218', title: 'Городские маршруты', category: 'Графы', level: 'Средняя', time: '35 мин', solved: '1.2k', color: 'blue' },
  { id: 'C-039', title: 'Последний рубеж', category: 'Динамика', level: 'Сложная', time: '50 мин', solved: '486', color: 'orange' }
];

export function filterTasks(items, query = '') {
  const normalized = query.trim().toLocaleLowerCase('ru');
  if (!normalized) return items;
  return items.filter((task) => `${task.title} ${task.category} ${task.id}`.toLocaleLowerCase('ru').includes(normalized));
}

