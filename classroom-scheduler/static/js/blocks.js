 // Initialize Flatpickr AFTER the DOM is fully loaded
  document.addEventListener("DOMContentLoaded", function() {
    flatpickr(".timepicker", {
      enableTime: true,
      noCalendar: true,
      dateFormat: "H:i",
      time_24hr: true,
      minuteIncrement: 5,
      allowInput: true,
    });
  });

  // Your existing auto-cascade script (unchanged)
  function updateSchedule() {
    const length = parseInt(document.getElementById('length').value) || 80;
    const breakM = parseInt(document.getElementById('break').value) || 10;
    let prevEnd = null;
    document.querySelectorAll('table tr').forEach((row, i) => {
      if (i === 0) return;
      const startInput = row.querySelector('input[name^="start_"]');
      const endInput = row.querySelector('input[name^="end_"]');
      if (!startInput) return;

      let startTime = startInput.value;
      if (!startTime && prevEnd) {
        const d = new Date(`2020-01-01 ${prevEnd}:00`);
        d.setMinutes(d.getMinutes() + breakM);
        startTime = d.toTimeString().slice(0,5);
        startInput.value = startTime;
        startInput._flatpickr.setDate(startTime, true);
      }

      if (startTime) {
        const [h, m] = startTime.split(':').map(Number);
        const d = new Date();
        d.setHours(h, m, 0, 0);
        d.setMinutes(d.getMinutes() + length);
        const endH = String(d.getHours()).padStart(2,'0');
        const endM = String(d.getMinutes()).padStart(2,'0');
        if (!endInput.value || endInput.value === endInput.defaultValue) {
          endInput.value = endH + ':' + endM;
          if (endInput._flatpickr) endInput._flatpickr.setDate(endH + ':' + endM, true);
        }
        prevEnd = endH + ':' + endM;
      }
    });
  }